// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/beevik/ntp"
	"github.com/ethereum/go-ethereum/common"
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
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
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

// ConsensusEngine defines the interface for consensus processing
type ConsensusEngine interface {
	Process(parentSummary *chain.BlockSummary, blk *block.Block, nowTimestamp uint64, blockConflicts uint32) (*state.Stage, tx.Receipts, error)
}

// PackerEngine defines the interface for packing blocks
type PackerEngine interface {
	Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (flow *packer.Flow, posActive bool, err error)
	SetTargetGasLimit(gl uint64)
}

type Node struct {
	packer      PackerEngine
	cons        ConsensusEngine
	master      *Master
	repo        *chain.Repository
	bft         *bft.Engine
	stater      *state.Stater
	logDB       *logdb.LogDB
	txPool      *txpool.TxPool
	txStashPath string
	comm        *comm.Communicator
	forkConfig  *thor.ForkConfig
	options     Options

	logDBFailed   bool
	initialSynced bool // true if the initial synchronization process is done
	bandwidth     bandwidth.Bandwidth
	maxBlockNum   uint32
	processLock   sync.Mutex
	logWorker     *worker
}

func New(
	master *Master,
	repo *chain.Repository,
	bft *bft.Engine,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	txStashPath string,
	comm *comm.Communicator,
	forkConfig *thor.ForkConfig,
	options Options,
	consensusEngine ConsensusEngine,
	packerEngine PackerEngine,
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
		comm:        comm,
		forkConfig:  forkConfig,
		options:     options,
	}
}

func (n *Node) Run(ctx context.Context) error {
	logWorker := newWorker()
	defer logWorker.Close()

	n.logWorker = logWorker

	maxBlockNum, err := n.repo.GetMaxBlockNum()
	if err != nil {
		return err
	}
	n.maxBlockNum = maxBlockNum

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

func (n *Node) houseKeeping(ctx context.Context) {
	logger.Debug("enter house keeping")
	defer logger.Debug("leave house keeping")

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockCh := make(chan *comm.NewBlockEvent)
	scope.Track(n.comm.SubscribeBlock(newBlockCh))

	futureTicker := time.NewTicker(time.Duration(thor.BlockInterval()) * time.Second)
	defer futureTicker.Stop()

	connectivityTicker := time.NewTicker(time.Second)
	defer connectivityTicker.Stop()

	clockSyncTicker := time.NewTicker(10 * time.Minute)
	defer clockSyncTicker.Stop()

	var noPeerTimes int

	futureBlocks := cache.NewRandCache(32)

	for {
		select {
		case <-ctx.Done():
			return
		case newBlock := <-newBlockCh:
			var stats blockStats
			if isTrunk, err := n.processBlock(newBlock.Block, &stats); err != nil {
				if consensus.IsFutureBlock(err) ||
					((err == errParentMissing || err == errBlockTemporaryUnprocessable) && futureBlocks.Contains(newBlock.Header().ParentID())) {
					logger.Debug("future block added", "id", newBlock.Header().ID())
					futureBlocks.Set(newBlock.Header().ID(), newBlock.Block)
				}
			} else if isTrunk {
				n.comm.BroadcastBlock(newBlock.Block)
				logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Header())...)
			}
		case <-futureTicker.C:
			// process future blocks
			var blocks []*block.Block
			futureBlocks.ForEach(func(ent *cache.Entry) bool {
				blocks = append(blocks, ent.Value.(*block.Block))
				return true
			})
			sort.Slice(blocks, func(i, j int) bool {
				return blocks[i].Header().Number() < blocks[j].Header().Number()
			})
			var stats blockStats
			for i, block := range blocks {
				if isTrunk, err := n.processBlock(block, &stats); err == nil || err == errKnownBlock {
					logger.Debug("future block consumed", "id", block.Header().ID())
					futureBlocks.Remove(block.Header().ID())
					if isTrunk {
						n.comm.BroadcastBlock(block)
					}
				}

				if stats.processed > 0 && i == len(blocks)-1 {
					logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
				}
			}
		case <-connectivityTicker.C:
			if n.comm.PeerCount() == 0 {
				noPeerTimes++
				if noPeerTimes > 30 {
					noPeerTimes = 0
					go checkClockOffset()
				}
			} else {
				noPeerTimes = 0
			}
		case <-clockSyncTicker.C:
			go checkClockOffset()
		}
	}
}

func (n *Node) txStashLoop(ctx context.Context) {
	logger.Debug("enter tx stash loop")
	defer logger.Debug("leave tx stash loop")

	db, err := leveldb.OpenFile(n.txStashPath, nil)
	if err != nil {
		logger.Error("create tx stash", "err", err)
		return
	}
	defer db.Close()

	stash := newTxStash(db, 1000)

	{
		txs := stash.LoadAll()
		n.txPool.Fill(txs)
		logger.Debug("loaded txs from stash", "count", len(txs))
	}

	var scope event.SubscriptionScope
	defer scope.Close()

	txCh := make(chan *txpool.TxEvent)
	scope.Track(n.txPool.SubscribeTxEvent(txCh))
	for {
		select {
		case <-ctx.Done():
			return
		case txEv := <-txCh:
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
