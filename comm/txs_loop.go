// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/txpool"
)

func (c *Communicator) txsLoop() {

	txEvCh := make(chan *txpool.TxEvent, 10)
	sub := c.txPool.SubscribeTxEvent(txEvCh)
	defer sub.Unsubscribe()

	for {
		select {
		case <-c.ctx.Done():
			return
		case txEv := <-txEvCh:
			if txEv.Executable != nil && *txEv.Executable {
				tx := txEv.Tx
				peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
					return !p.IsTransactionKnown(tx.Hash())
				})

				for _, peer := range peers {
					peer := peer
					peer.MarkTransaction(tx.Hash())
					c.goes.Go(func() {
						if err := proto.NotifyNewTx(c.ctx, peer, tx); err != nil {
							peer.logger.Debug("failed to broadcast tx", "err", err)
						}
					})
				}
			}
		}
	}
}
