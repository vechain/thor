package txpool

import (
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const maxTxSize = 32 * 1024 // Reject transactions over 32KB to prevent DOS attacks

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
	rw     sync.RWMutex
	txFeed event.Feed
	scope  event.SubscriptionScope

	data struct {
		dirty   bool
		all     *cache.RandCache
		pending txObjects
		sorted  bool
	}
}

//New construct a new txpool
func New(chain *chain.Chain, stateC *state.Creator) *TxPool {
	pool := &TxPool{
		config: defaultTxPoolConfig,
		chain:  chain,
		stateC: stateC,
		done:   make(chan struct{}),
	}
	pool.data.all = cache.NewRandCache(pool.config.PoolSize)
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
	pool.rw.Lock()
	defer pool.rw.Unlock()

	for _, tx := range txs {
		txID := tx.ID()
		if _, ok := pool.data.all.Get(txID); ok {
			return errKnownTx
		}

		// If the transaction fails basic validation, discard it
		if err := pool.validateTx(tx); err != nil {
			return err
		}

		if pool.data.all.Len() > pool.config.PoolSize {
			if picked, ok := pool.data.all.Pick().Value.(*txObject); ok {
				pool.data.all.Remove(picked.tx.ID())
			}
		}

		pool.data.all.Set(txID, &txObject{
			tx:           tx,
			overallGP:    new(big.Int),
			creationTime: time.Now().Unix(),
			status:       Queued,
		})

		pool.data.dirty = true
		pool.goes.Go(func() { pool.txFeed.Send(tx) })
	}

	return nil
}

//Remove remove transaction by txID with TransactionCategory
func (pool *TxPool) Remove(txIDs ...thor.Bytes32) {
	pool.rw.Lock()
	defer pool.rw.Unlock()

	for _, txID := range txIDs {
		if value, ok := pool.data.all.Get(txID); ok {
			if obj, ok := value.(*txObject); ok {
				pool.data.all.Remove(txID)
				obj.deleted = true
			}
		}
	}
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan *tx.Transaction) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

//Dump dump transactions by TransactionCategory
func (pool *TxPool) Dump() tx.Transactions {
	return pool.pending(false).parseTxs()
}

//Pending return all pending txs
func (pool *TxPool) Pending() tx.Transactions {
	return pool.pending(true).parseTxs()
}

func (pool *TxPool) pending(shouldSort bool) txObjects {
	if pool.data.dirty {
		pool.updateData(pool.chain.BestBlock())
	}

	pending := pool.dumpPending()
	if shouldSort && !pool.data.sorted {
		sort.Slice(pending, func(i, j int) bool {
			return pending[i].overallGP.Cmp(pending[j].overallGP) > 0
		})
		pool.data.sorted = true
	}

	return pending
}

func (pool *TxPool) dumpPending() txObjects {
	pool.rw.Lock()
	defer pool.rw.Unlock()

	size := len(pool.data.pending)
	pending := make(txObjects, size, size)

	for i, obj := range pool.data.pending {
		pending[i] = obj
	}

	return pending
}

func (pool *TxPool) validateTx(tx *tx.Transaction) error {
	if tx.Size() > maxTxSize {
		return errTooLarge
	}

	bestBlock := pool.chain.BestBlock()

	if tx.IsExpired(bestBlock.Header().Number()) {
		return errExpired
	}

	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return err
	}

	resolvedTx, err := runtime.ResolveTransaction(st, tx)
	if err != nil {
		return errIntrisicGasExceeded
	}

	_, _, err = resolvedTx.BuyGas(bestBlock.Header().Number() + 1)
	if err != nil {
		return errInsufficientEnergy
	}

	for _, clause := range resolvedTx.Clauses {
		if clause.Value().Sign() < 0 {
			return errNegativeValue
		}
	}

	return nil
}
