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
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/thor"
)

func (n *Node) houseKeeping(ctx context.Context) {
	logger.Debug("enter house keeping")

	connectivity := new(ConnectivityState)
	futureTicker := time.NewTicker(time.Duration(thor.BlockInterval()) * time.Second)
	connectivityTicker := time.NewTicker(time.Second)
	clockSyncTicker := time.NewTicker(10 * time.Minute)

	defer func() {
		logger.Debug("leave house keeping")
		futureTicker.Stop()
		connectivityTicker.Stop()
		clockSyncTicker.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case newBlock := <-n.newBlockCh:
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

func (n *Node) handleNewBlock(newBlockEvent *comm.NewBlockEvent) {
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
