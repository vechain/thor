// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/beevik/ntp"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/p2p/discover"
	"github.com/vechain/thor/v2/thor"
)

func (n *Node) houseKeeping(ctx context.Context) {
	logger.Debug("enter house keeping")

	var scope event.SubscriptionScope

	newBlockCh := make(chan *comm.NewBlockEvent)
	scope.Track(n.comm.SubscribeBlock(newBlockCh))

	connectivity := new(ConnectivityState)
	futureTicker := time.NewTicker(time.Duration(thor.BlockInterval()) * time.Second)
	connectivityTicker := time.NewTicker(time.Second)
	clockSyncTicker := time.NewTicker(10 * time.Minute)

	defer func() {
		logger.Debug("leave house keeping")
		scope.Close()
		futureTicker.Stop()
		connectivityTicker.Stop()
		clockSyncTicker.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case newBlock := <-newBlockCh:
			n.handleNewBlock(newBlock)
		case <-futureTicker.C:
			n.handleFutureBlocks()
		case <-connectivityTicker.C:
			connectivity.Check(n.comm.PeerCount())
		case <-clockSyncTicker.C:
			go checkClockOffset()
		}
	}
}

// futureBlockWindow caps how far ahead a block may be, both in time and in gas
// limit drift over the chain head. The handshake caps peer clock skew at two
// block intervals; malicious timestamps have no bound at all.
const futureBlockWindow = 4

// maxFutureBlocksPerPeer caps the future block cache entries one peer may
// hold, so a single peer cannot evict everyone else's. An honest peer can
// supply at most one block per height across the window, plus forks.
const maxFutureBlocksPerPeer = 2 * futureBlockWindow

// futureBlock is a future block cache entry.
type futureBlock struct {
	blk    *block.Block
	origin discover.NodeID
}

func (n *Node) handleNewBlock(newBlockEvent *comm.NewBlockEvent) {
	var stats blockStats
	newBlock := newBlockEvent.Block
	if isTrunk, err := n.processBlock(newBlock, &stats); err != nil {
		if consensus.IsFutureBlock(err) ||
			((err == errParentMissing || err == errBlockTemporaryUnprocessable) && n.futureBlocksCache.Contains(newBlock.Header().ParentID())) {
			// Keep the first copy of an ID. Its txs root binds its body to the ID,
			// but the ID does not cover the VRF proof: a copy with a forged proof
			// that arrives first fails consensus later, and the honest block then
			// comes from another peer or the next sync round. Verifying proofs
			// here would cost a VRF verify per copy.
			if n.futureBlocksCache.Contains(newBlock.Header().ID()) || !n.shouldCacheFutureBlock(newBlock) {
				return
			}
			if n.countFutureBlocksFrom(newBlockEvent.Origin) >= maxFutureBlocksPerPeer {
				logger.Debug("future block rejected, peer holds too many", "id", newBlock.Header().ID(), "peer", newBlockEvent.Origin)
				return
			}
			logger.Debug("future block added", "id", newBlock.Header().ID())
			n.futureBlocksCache.Set(newBlock.Header().ID(), &futureBlock{blk: newBlock, origin: newBlockEvent.Origin})
		}
	} else if isTrunk {
		n.comm.BroadcastBlock(newBlock)
		logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Header())...)
	}
}

// shouldCacheFutureBlock reports whether a block may enter the future block
// cache. The gas limit ceiling comes from the chain head, not the parent: a
// chained future block's parent is itself an unverified cache entry.
func (n *Node) shouldCacheFutureBlock(blk *block.Block) bool {
	header := blk.Header()

	if header.Timestamp() > uint64(time.Now().Unix())+futureBlockWindow*thor.BlockInterval() {
		logger.Debug("future block rejected, timestamp too far ahead", "id", header.ID(), "timestamp", header.Timestamp())
		return false
	}

	// Gas limit may drift 1/GasLimitBoundDivisor per block, so allow the whole window.
	ceiling := n.repo.BestBlockSummary().Header.GasLimit()
	for range futureBlockWindow {
		ceiling += ceiling / thor.GasLimitBoundDivisor
	}

	if err := n.cons.PrevalidateStateless(blk, ceiling); err != nil {
		logger.Debug("future block rejected", "id", header.ID(), "err", err)
		return false
	}

	return true
}

// countFutureBlocksFrom returns how many future block cache entries came from origin.
func (n *Node) countFutureBlocksFrom(origin discover.NodeID) int {
	count := 0
	n.futureBlocksCache.ForEach(func(ent *cache.Entry) bool {
		if ent.Value.(*futureBlock).origin == origin {
			count++
		}
		return true
	})
	return count
}

func (n *Node) handleFutureBlocks() {
	var blocks []*block.Block
	n.futureBlocksCache.ForEach(func(ent *cache.Entry) bool {
		blocks = append(blocks, ent.Value.(*futureBlock).blk)
		return true
	})
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Header().Number() < blocks[j].Header().Number()
	})
	var stats blockStats
	for i, blk := range blocks {
		isTrunk, err := n.processBlock(blk, &stats)
		switch {
		case err == nil || err == errKnownBlock:
			logger.Debug("future block consumed", "id", blk.Header().ID())
			n.futureBlocksCache.Remove(blk.Header().ID())
			if isTrunk {
				n.comm.BroadcastBlock(blk)
			}
		case consensus.IsFutureBlock(err) || err == errParentMissing || err == errBlockTemporaryUnprocessable:
			// Only errFutureBlock is self-limiting; an orphan whose parent got
			// evicted waits forever, so age it out on the admission window.
			if blk.Header().Timestamp()+futureBlockWindow*thor.BlockInterval() < uint64(time.Now().Unix()) {
				logger.Debug("future block aged out", "id", blk.Header().ID())
				n.futureBlocksCache.Remove(blk.Header().ID())
			}
		default:
			// Consensus failures, plus I/O errors from executeAndCommitBlock.
			logger.Debug("future block dropped", "id", blk.Header().ID(), "err", err)
			n.futureBlocksCache.Remove(blk.Header().ID())
		}

		if stats.processed > 0 && i == len(blocks)-1 {
			logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(blk.Header())...)
		}
	}
}

type ConnectivityState uint

func (c *ConnectivityState) Check(peers int) {
	if peers == 0 {
		*c++
		if *c > 30 {
			*c = 0
			go checkClockOffset()
		}
	} else {
		*c = 0
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
