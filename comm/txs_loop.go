package comm

import (
	"math"

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
			p := int(math.Sqrt(float64(len(peers))))
			toPropagate := peers[:p]

			for _, peer := range toPropagate {
				peer := peer
				peer.MarkTransaction(tx.ID())
				c.goes.Go(func() {
					if err := proto.NotifyNewTx(c.ctx, peer, tx); err != nil {
						peer.logger.Debug("failed to broadcast tx", "err", err)
					}
				})
			}
		}
	}
}
