package main

import (
	"context"

	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/tx"
	Txpool "github.com/vechain/thor/txpool"
)

type txRoutineContext struct {
	ctx          context.Context
	communicator *comm.Communicator
	txpool       *Txpool.TxPool
}

func synchronizeTx(context *txRoutineContext) {
	var goes co.Goes

	goes.Go(func() {
		txBroadcastLoop(context)
	})
	goes.Go(func() {
		txPoolUpdateLoop(context)
	})

	log.Info("Transaction Synchronization started")
	goes.Wait()
	log.Info("Transaction Synchronization stoped")
}

func txBroadcastLoop(context *txRoutineContext) {
	txCh := make(chan *tx.Transaction)
	sub := context.txpool.SubscribeNewTransaction(txCh)

	for {
		select {
		case <-context.ctx.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			context.communicator.BroadcastTx(tx)
		}
	}
}

func txPoolUpdateLoop(context *txRoutineContext) {
	txCh := make(chan *tx.Transaction)
	sub := context.communicator.SubscribeTx(txCh)

	for {
		select {
		case <-context.ctx.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			context.txpool.Add(tx)
		}
	}
}
