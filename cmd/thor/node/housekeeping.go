// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"sort"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/consensus"
)

func (n *Node) houseKeeping(ctx context.Context) {
	logger.Debug("enter house keeping")
	defer logger.Debug("leave house keeping")

	var noPeerTimes int

	for {
		select {
		case <-ctx.Done():
			logger.Debug("received context done signal")
			return
		case newBlock := <-n.newBlockCh:
			logger.Debug("received new block signal")
			var stats blockStats
			if isTrunk, err := n.processBlock(newBlock.Block, &stats); err != nil {
				if consensus.IsFutureBlock(err) ||
					((err == errParentMissing || err == errBlockTemporaryUnprocessable) && n.futureBlocksCache.Contains(newBlock.Header().ParentID())) {
					logger.Debug("future block added", "id", newBlock.Header().ID())
					n.futureBlocksCache.Set(newBlock.Header().ID(), newBlock.Block)
				}
			} else if isTrunk {
				n.comm.BroadcastBlock(newBlock.Block)
				logger.Info(fmt.Sprintf("imported blocks (%v)", stats.processed), stats.LogContext(newBlock.Header())...)
			}
		case <-n.futureTicker.C:
			// process future blocks
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
		case <-n.connectivityTicker.C:
			logger.Debug("received connectivity tick")
			if n.comm.PeerCount() == 0 {
				noPeerTimes++
				if noPeerTimes > 30 {
					noPeerTimes = 0
					go checkClockOffset()
				}
			} else {
				noPeerTimes = 0
			}
		case <-n.clockSyncTicker.C:
			logger.Debug("received clock sync tick>>")
			go checkClockOffset()
		}
	}
}
