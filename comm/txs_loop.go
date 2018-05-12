package comm

import (
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/tx"
)

func (c *Communicator) txsLoop() {

	txCh := make(chan *tx.Transaction)
	sub := c.txPool.SubscribeNewTransaction(txCh)
	defer sub.Unsubscribe()

	for {
		select {
		case <-c.ctx.Done():
			return
		case tx := <-txCh:
			peers := c.peerSet.Slice().Filter(func(p *Peer) bool {
				return !p.IsTransactionKnown(tx.ID())
			})

			for _, peer := range peers {
				peer := peer
				peer.MarkTransaction(tx.ID())
				c.goes.Go(func() {
					if err := (proto.NewTx{Tx: tx}.Call(c.ctx, peer)); err != nil {
						peer.logger.Debug("failed to broadcast tx", "err", err)
					}
				})
			}
		}
	}
}
