// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/beevik/ntp"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/cmd/thor/bandwidth"
	"github.com/vechain/thor/v2/co"
	comm2 "github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

var logger = log.WithContext("pkg", "node")

var (
	// error when the block larger than known max block number + 1
	errBlockTemporaryUnprocessable = errors.New("block temporary unprocessable")
	errKnownBlock                  = errors.New("block already in the chain")
	errParentMissing               = errors.New("parent block is missing")
	errBFTRejected                 = errors.New("block rejected by BFT engine")
)

// Options options for tx pool.
type Options struct {
	TargetGasLimit   uint64
	SkipLogs         bool
	MinTxPriorityFee uint64
}

type Node struct {
	packer      PackerEngine
	cons        ConsensusEngine
	master      *Master
	repo        RepositoryEngine
	bft         BFTEngine
	stater      *state.Stater
	logDB       *logdb.LogDB
	txPool      TxPoolEngine
	txStashPath string
	comm        CommunicatorEngine
	forkConfig  *thor.ForkConfig
	options     Options

	logDBFailed       bool
	initialSynced     bool // true if the initial synchronization process is done
	bandwidth         bandwidth.Bandwidth
	maxBlockNum       uint32
	processLock       sync.Mutex
	logWorker         *worker
	scope             event.SubscriptionScope
	newBlockCh        chan *comm2.NewBlockEvent
	txCh              chan *txpool.TxEvent
	futureBlocksCache *cache.RandCache
	txStash           *txStash
}

func New(
	master *Master,
	repo RepositoryEngine,
	bft BFTEngine,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool TxPoolEngine,
	txStashPath string,
	comm CommunicatorEngine,
	forkConfig *thor.ForkConfig,
	options Options,
	consensusEngine ConsensusEngine,
	packerEngine PackerEngine,
) *Node {
	n := &Node{
		packer:      packerEngine,
		cons:        consensusEngine,
		master:      master,
		repo:        repo,
		bft:         bft,
		stater:      stater,
		logDB:       logDB,
		txPool:      txPool,
		txStashPath: txStashPath,
		comm:        comm,
		forkConfig:  forkConfig,
		options:     options,
	}

	// initialize members
	n.logWorker = newWorker()
	n.futureBlocksCache = cache.NewRandCache(32)
	n.scope = event.SubscriptionScope{}

	n.newBlockCh = make(chan *comm2.NewBlockEvent)
	n.scope.Track(n.comm.SubscribeBlock(n.newBlockCh))

	n.txCh = make(chan *txpool.TxEvent)
	n.scope.Track(n.txPool.SubscribeTxEvent(n.txCh))

	return n
}

func (n *Node) Run(ctx context.Context) error {
	defer n.logWorker.Close()
	defer n.scope.Close()

	maxBlockNum, err := n.repo.GetMaxBlockNum()
	if err != nil {
		return err
	}
	n.maxBlockNum = maxBlockNum

	db, err := leveldb.OpenFile(n.txStashPath, nil)
	if err != nil {
		logger.Error("create tx stash", "err", err)
		return err
	}
	defer db.Close()

	n.txStash = newTxStash(db, 1000)

	var goes co.Goes
	goes.Go(func() { n.comm.Sync(ctx, n.handleBlockStream) })
	goes.Go(func() { n.houseKeeping(ctx) })
	goes.Go(func() { n.txStashLoop(ctx) })
	goes.Go(func() { n.packerLoop(ctx) })

	goes.Wait()
	return nil
}

func (n *Node) handleBlockStream(ctx context.Context, stream <-chan *block.Block) (err error) {
	logger.Debug("start to process block stream")
	defer logger.Debug("process block stream done", "err", err)
	var stats blockStats
	startTime := mclock.Now()

	report := func(block *block.Block) {
		logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
		stats = blockStats{}
		startTime = mclock.Now()
	}

	var blk *block.Block
	for blk = range stream {
		if blk == nil {
			logger.Debug("received nil from stream")
			continue
		}
		if _, err := n.processBlock(blk, &stats); err != nil {
			return err
		}

		if stats.processed > 0 &&
			mclock.Now()-startTime > mclock.AbsTime(time.Second*2) {
			report(blk)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if blk != nil && stats.processed > 0 {
		report(blk)
	}
	return nil
}

func checkClockOffset() {
	resp, err := ntp.Query("pool.ntp.org")
	if err != nil {
		logger.Debug("failed to access NTP", "err", err)
		return
	}
	if resp.ClockOffset > time.Duration(thor.BlockInterval())*time.Second/2 {
		logger.Warn("clock offset detected", "offset", common.PrettyDuration(resp.ClockOffset))
	}
}
