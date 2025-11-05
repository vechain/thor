// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	comm2 "github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/thor"
)

func (n *Node) houseKeeping(ctx context.Context) {
	logger.Debug("enter house keeping")

	var noPeerTimes int
	futureTicker := time.NewTicker(time.Duration(thor.BlockInterval()) * time.Second)
	connectivityTicker := time.NewTicker(time.Second)
	clockSyncTicker := time.NewTicker(10 * time.Minute)

	defer func() {
		futureTicker.Stop()
		connectivityTicker.Stop()
		clockSyncTicker.Stop()
		logger.Debug("leave house keeping")
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("received context done signal")
			return
		case newBlock := <-n.newBlockCh:
			n.handleNewBlock(newBlock)
		case <-futureTicker.C:
			n.handleFutureBlocks()
		case <-connectivityTicker.C:
			n.handleCconnectivityTicker(&noPeerTimes)
		case <-clockSyncTicker.C:
			n.handleClockSyncTick()
		}
	}
}

func (n *Node) handleNewBlock(newBlockEvent *comm2.NewBlockEvent) {
	logger.Debug("received new block")
	if newBlockEvent == nil || newBlockEvent.Block == nil {
		logger.Debug("Received nil block event")
		return
	}

	var stats blockStats
	newBlock := newBlockEvent.Block
	if isTrunk, err := n.processBlock(newBlock, &stats); err != nil {
		if consensus.IsFutureBlock(err) ||
			((err == errParentMissing || err == errBlockTemporaryUnprocessable) && n.futureBlocksCache.Contains(newBlock.Header().ParentID())) {
			logger.Debug("future block added", "id", newBlock.Header().ID())
			n.futureBlocksCache.Set(newBlock.Header().ID(), newBlock)
		}
	} else if isTrunk {
		n.comm.BroadcastBlock(newBlock)
		logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Header())...)
	}
}

func (n *Node) handleFutureBlocks() {
	logger.Debug("received future block signal")
	var blocks []*block.Block
	n.futureBlocksCache.ForEach(func(ent *cache.Entry) bool {
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
			n.futureBlocksCache.Remove(block.Header().ID())
			if isTrunk {
				n.comm.BroadcastBlock(block)
			}
		}

		if stats.processed > 0 && i == len(blocks)-1 {
			logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(block.Header())...)
		}
	}
}

func (n *Node) handleCconnectivityTicker(noPeerTimes *int) {
	logger.Debug("received connectivity tick")
	if n.comm.PeerCount() == 0 {
		logger.Debug("no peers connected")
		*noPeerTimes++
		if *noPeerTimes > 30 {
			*noPeerTimes = 0
			go checkClockOffset()
		}
	} else {
		logger.Debug("have peers connected")
		*noPeerTimes = 0
	}
}

func (n *Node) handleClockSyncTick() {
	logger.Debug("received clock sync tick")
	go checkClockOffset()
}
