// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
)

func (n *Node) txStashLoop(ctx context.Context) {
	logger.Debug("enter tx stash loop")
	defer logger.Debug("leave tx stash loop")

	{
		txs := n.txStash.LoadAll()
		if len(txs) > 0 {
			n.txPool.Fill(txs)
		}
		logger.Debug("loaded txs from stash", "count", len(txs))
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("received context done signal")
			return
		case txEv := <-n.txCh:
			logger.Debug("received tx signal")
			// skip executables
			if txEv.Executable != nil && *txEv.Executable {
				logger.Debug("received executable tx signal")
				continue
			}
			// only stash non-executable txs
			if err := n.txStash.Save(txEv.Tx); err != nil {
				logger.Warn("stash tx", "id", txEv.Tx.ID().String(), "err", err)
			} else {
				logger.Trace("stashed tx", "id", txEv.Tx.ID().String())
			}
		}
	}
}
