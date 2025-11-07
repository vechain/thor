// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/bandwidth"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/comm"
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
	packer      Packer
	cons        Consensus
	master      *Master
	repo        *chain.Repository
	bft         bft.Committer
	stater      *state.Stater
	logDB       *logdb.LogDB
	txPool      txpool.Pool
	txStashPath string
	comm        Communicator
	forkConfig  *thor.ForkConfig
	options     Options

	logDBFailed       bool
	initialSynced     bool // true if the initial synchronization process is done
	bandwidth         bandwidth.Bandwidth
	maxBlockNum       uint32
	processLock       sync.Mutex
	logWorker         *worker
	scope             event.SubscriptionScope
	newBlockCh        chan *comm.NewBlockEvent
	txCh              chan *txpool.TxEvent
	futureBlocksCache *cache.RandCache
}

func New(
	master *Master,
	repo *chain.Repository,
	bft bft.Committer,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool txpool.Pool,
	txStashPath string,
	communicator Communicator,
	forkConfig *thor.ForkConfig,
	options Options,
	consensusEngine Consensus,
	packerEngine Packer,
) *Node {
	return &Node{
		packer:      packerEngine,
		cons:        consensusEngine,
		master:      master,
		repo:        repo,
		bft:         bft,
		stater:      stater,
		logDB:       logDB,
		txPool:      txPool,
		txStashPath: txStashPath,
		comm:        communicator,
		forkConfig:  forkConfig,
		options:     options,

		logWorker:         newWorker(),
		futureBlocksCache: cache.NewRandCache(32),
		scope:             event.SubscriptionScope{},
	}
}

func (n *Node) Run(ctx context.Context) error {
	defer n.logWorker.Close()
	defer n.scope.Close()

	n.newBlockCh = make(chan *comm.NewBlockEvent)
	n.scope.Track(n.comm.SubscribeBlock(n.newBlockCh))

	n.txCh = make(chan *txpool.TxEvent)
	n.scope.Track(n.txPool.SubscribeTxEvent(n.txCh))

	maxBlockNum, err := n.repo.GetMaxBlockNum()
	if err != nil {
		return err
	}
	n.maxBlockNum = maxBlockNum

	db, err := leveldb.OpenFile(n.txStashPath, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	txStash := newTxStash(db, 1000)

	var goes co.Goes
	goes.Go(func() { n.comm.Sync(ctx, n.handleBlockStream) })
	goes.Go(func() { n.houseKeeping(ctx) })
	goes.Go(func() { n.txStashLoop(ctx, txStash) })
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
		// block stream will have nil for throttling, just skip it
		if blk == nil {
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

func (n *Node) txStashLoop(ctx context.Context, stash *txStash) {
	logger.Debug("enter tx stash loop")
	defer logger.Debug("leave tx stash loop")

	{
		txs := stash.LoadAll()
		if len(txs) > 0 {
			n.txPool.Fill(txs)
		}
		logger.Debug("loaded txs from stash", "count", len(txs))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case txEv := <-n.txCh:
			// skip executables
			if txEv.Executable != nil && *txEv.Executable {
				continue
			}
			// only stash non-executable txs
			if err := stash.Save(txEv.Tx); err != nil {
				logger.Warn("stash tx", "id", txEv.Tx.ID(), "err", err)
			} else {
				logger.Trace("stashed tx", "id", txEv.Tx.ID())
			}
		}
	}
}
