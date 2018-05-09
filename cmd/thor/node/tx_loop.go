package node

import (
	"context"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/tx"
)

func (n *Node) txLoop(ctx context.Context) {
	log.Debug("enter tx loop")
	defer log.Debug("leave tx loop")

	var scope event.SubscriptionScope
	defer scope.Close()

	txPoolCh := make(chan *tx.Transaction)
	commTxCh := make(chan *comm.NewTransactionEvent)

	scope.Track(n.txPool.SubscribeNewTransaction(txPoolCh))
	scope.Track(n.comm.SubscribeTransaction(commTxCh))

	for {
		select {
		case <-ctx.Done():
			return
		case tx := <-txPoolCh:
			n.comm.BroadcastTx(tx)
		case tx := <-commTxCh:
			if err := n.txPool.Add(tx.Transaction); err != nil {
				log.Debug("failed to add tx to tx pool", "err", err, "id", tx.ID())
			}
		}
	}
}
