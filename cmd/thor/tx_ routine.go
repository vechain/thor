package main

import (
	"context"

	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/tx"
	Txpool "github.com/vechain/thor/txpool"
)

func broadcastTxLoop(ctx context.Context, communicator *comm.Communicator, txpool *Txpool.TxPool) {
	txCh := make(chan *tx.Transaction)
	sub := txpool.SubscribeNewTransaction(txCh)

	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			communicator.BroadcastTx(tx)
		}
	}
}

func txPoolUpdateLoop(ctx context.Context, communicator *comm.Communicator, txpool *Txpool.TxPool) {
	txCh := make(chan *tx.Transaction)
	sub := communicator.SubscribeTx(txCh)

	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			txpool.Add(tx)
		}
	}
}
