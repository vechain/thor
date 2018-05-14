package txpool

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const maxTxSize = 32 * 1024 // Reject transactions over 32KB to prevent DOS attacks
const quotaSignerTx = 100   // Each signer saves up to 100 txs

//PoolConfig PoolConfig
type PoolConfig struct {
	PoolSize int           // Maximum number of executable transaction slots for all accounts
	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

//DefaultTxPoolConfig DefaultTxPoolConfig
var defaultTxPoolConfig = PoolConfig{
	PoolSize: 20000,
	Lifetime: 1000,
}

//TxPool TxPool
type TxPool struct {
	config PoolConfig
	chain  *chain.Chain
	stateC *state.Creator
	goes   co.Goes
	done   chan struct{}
	txFeed event.Feed
	scope  event.SubscriptionScope
	entry  *entry
}

//New construct a new txpool
func New(chain *chain.Chain, stateC *state.Creator) *TxPool {
	pool := &TxPool{
		config: defaultTxPoolConfig,
		chain:  chain,
		stateC: stateC,
		done:   make(chan struct{}),
	}
	pool.entry = newEntry(pool.config.PoolSize)
	pool.goes.Go(pool.updateLoop)
	return pool
}

//Close close pool loop
func (pool *TxPool) Close() {
	close(pool.done)
	pool.scope.Close()
	pool.goes.Wait()
}

//Add transaction
func (pool *TxPool) Add(txs ...*tx.Transaction) error {
	for _, tx := range txs {
		tx := tx // it's for closure
		txID := tx.ID()
		if obj := pool.entry.find(txID); obj != nil {
			return rejectedTx("known transaction")
		}

		// If the transaction fails basic validation, discard it
		signer, err := pool.validateTx(tx)
		if err != nil {
			return err
		}

		if pool.entry.size() > pool.config.PoolSize {
			pool.entry.pick()
		}

		pool.entry.save(&txObject{
			tx:           tx,
			signer:       signer,
			overallGP:    new(big.Int),
			creationTime: time.Now().Unix(),
			status:       Queued,
		})
		pool.goes.Go(func() { pool.txFeed.Send(tx) })
	}

	return nil
}

//Remove remove transaction by txID with TransactionCategory
func (pool *TxPool) Remove(txIDs ...thor.Bytes32) {
	for _, txID := range txIDs {
		pool.entry.delete(txID)
	}
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan *tx.Transaction) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

//Pending return all pending txs
func (pool *TxPool) Pending(sort bool) tx.Transactions {
	if pool.entry.isDirty() {
		pool.updateData(pool.chain.BestBlock())
	}
	return pool.entry.dumpPending(sort).parseTxs()
}

func (pool *TxPool) validateTx(tx *tx.Transaction) (thor.Address, error) {
	if tx.Size() > maxTxSize {
		return thor.Address{}, rejectedTx("tx too large")
	}

	if tx.ChainTag() != pool.chain.Tag() {
		return thor.Address{}, badTx("chain tag mismatched")
	}

	if tx.HasReservedFields() {
		return thor.Address{}, badTx("reserved fields not empty")
	}

	bestBlock := pool.chain.BestBlock()

	if tx.Gas() > bestBlock.Header().GasLimit() {
		return thor.Address{}, badTx("tx gas exceeded")
	}

	if tx.IsExpired(bestBlock.Header().Number()) {
		return thor.Address{}, rejectedTx("tx expired")
	}

	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return thor.Address{}, err
	}

	resolvedTx, err := runtime.ResolveTransaction(st, tx)
	if err != nil {
		return thor.Address{}, badTx("intrinsic gas exceeds provided gas")
	}

	if pool.entry.quotaBySinger(resolvedTx.Origin) >= quotaSignerTx {
		return thor.Address{}, rejectedTx("quota exceeds limit")
	}

	_, _, err = resolvedTx.BuyGas(st, bestBlock.Header().Number()+1)
	if err != nil {
		return thor.Address{}, rejectedTx("insufficient energy")
	}

	for _, clause := range resolvedTx.Clauses {
		if clause.Value().Sign() < 0 {
			return thor.Address{}, badTx("negative clause value")
		}
	}

	return resolvedTx.Origin, nil
}
