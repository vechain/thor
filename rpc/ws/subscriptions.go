// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ws

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

const pendingTxBufSize = 128

// runNewHeads pushes an EthBlock notification (fullTxs=false) for every new
// canonical block while ctx is alive. Obsolete (reorg) blocks are skipped
// because newHeads delivers only the canonical chain tip.
func runNewHeads(ctx context.Context, c *wsConn, subID string) {
	reader := c.repo.NewBlockReader(c.repo.BestBlockSummary().Header.ID())
	ticker := c.repo.NewTicker()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
		}
		for {
			blocks, err := reader.Read()
			if err != nil || len(blocks) == 0 {
				break
			}
			for _, blk := range blocks {
				if blk.Obsolete {
					continue // deliver canonical tips only
				}
				ethBlock, err := ethconvert.BuildEthBlock(blk.Header(), c.repo, false)
				if err != nil {
					continue
				}
				c.notify(subID, ethBlock)
			}
		}
	}
}

// runLogs pushes EthLog notifications for every new block while ctx is alive.
// For canonical (non-obsolete) blocks, logs are pushed with Removed=false.
// For obsolete blocks (reorg), the same logs are re-emitted with Removed=true
// so subscribers can roll back their state — per the Ethereum eth_subscribe spec.
func runLogs(ctx context.Context, c *wsConn, subID string, criteria ethconvert.LogCriteria) {
	reader := c.repo.NewBlockReader(c.repo.BestBlockSummary().Header.ID())
	ticker := c.repo.NewTicker()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
		}
		for {
			blocks, err := reader.Read()
			if err != nil || len(blocks) == 0 {
				break
			}
			for _, blk := range blocks {
				receipts, err := c.repo.GetBlockReceipts(blk.Header().ID())
				if err != nil {
					continue
				}
				// Obsolete=true means this block was part of a fork that got replaced.
				// We re-emit its logs with removed=true so subscribers can undo their state.
				removed := blk.Obsolete
				logs := ethconvert.CollectMatchingLogs(
					&criteria,
					blk.Transactions(),
					receipts,
					common.Hash(blk.Header().ID()),
					uint64(blk.Header().Number()),
					removed,
				)
				for _, log := range logs {
					c.notify(subID, log)
				}
			}
		}
	}
}

// runNewPendingTransactions pushes the transaction hash for every executable
// ETH-typed (TypeEthDynamicFee) transaction that enters the pool while ctx is alive.
// VeChain-native transactions are intentionally excluded: Ethereum tooling cannot
// decode or display them and their IDs do not match any Ethereum tx format.
func runNewPendingTransactions(ctx context.Context, c *wsConn, subID string) {
	txCh := make(chan *txpool.TxEvent, pendingTxBufSize)
	sub := c.txPool.SubscribeTxEvent(txCh)
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-txCh:
			if !ok {
				return
			}
			// Only report executable ETH-typed transactions.
			if ev.Executable == nil || !*ev.Executable {
				continue
			}
			if ev.Tx.Type() != tx.TypeEthDynamicFee {
				continue
			}
			c.notify(subID, common.Hash(ev.Tx.ID()))
		case <-time.After(pongWait * time.Second):
			// Safety valve: if txCh produces nothing for a full pong cycle,
			// loop back so connCtx.Done() is checked.
		}
	}
}
