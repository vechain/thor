package txpool

import (
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
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
	return pool
}

// Start start txpool loop
func (pool *TxPool) Start(ch chan *block.Block) {
	pool.goes.Go(func() {
		for {
			select {
			case <-pool.done:
				return
			case bestBlock := <-ch:
				pool.updateData(bestBlock)
			}
		}
	})
}

//Stop Stop pool loop
func (pool *TxPool) Stop() {
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
		})

		pool.data.pending = nil
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
	if pool.data.pending == nil {
		pool.updateData(pool.chain.BestBlock())
	}

	if shouldSort && !pool.data.sorted {
		sort.Slice(pool.data.pending, func(i, j int) bool {
			return pool.data.pending[i].overallGP.Cmp(pool.data.pending[j].overallGP) > 0
		})
		pool.data.sorted = true
	}

	return pool.data.pending
}

func (pool *TxPool) updateData(bestBlock *block.Block) {
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return
	}

	baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())

	pool.rw.Lock()
	allObjs := make(map[thor.Bytes32]*txObject)
	pool.data.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			if key, ok := entry.Key.(thor.Bytes32); ok {
				allObjs[key] = obj
				return true
			}
		}
		return false
	})
	pool.rw.Unlock()

	status := func(tx *tx.Transaction) (objectStatus, error) {
		dependsOn := tx.DependsOn()
		if dependsOn != nil {
			if _, _, err := pool.chain.GetTransaction(*dependsOn); err != nil {
				if pool.chain.IsNotFound(err) {
					return Queued, nil
				}
				return Queued, err
			}
		}
		nextBlockNum := bestBlock.Header().Number() + 1
		if tx.BlockRef().Number() > nextBlockNum {
			return Queued, nil
		}
		return Pending, nil
	}

	pool.data.pending = make(txObjects, 0, len(allObjs))
	pool.data.sorted = false

	//can be pendinged txObjects
	for id, obj := range allObjs {
		if obj.tx.IsExpired(bestBlock.Header().Number()) || time.Now().Unix()-obj.creationTime > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}

		if obj.status == Queued {
			state, err := status(obj.tx)
			if err != nil {
				return
			}
			if state == Pending {
				overallGP := obj.tx.OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Bytes32 {
					return traverser.Get(num).ID()
				})
				obj.overallGP = overallGP
				obj.status = Pending
				pool.data.all.Set(id, obj)
			}
		}

		if obj.status == Pending {
			pool.data.pending = append(pool.data.pending, obj)
		}
	}
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
