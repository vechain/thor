package txpool

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	// ErrIntrinsicGas intrinsic gas too low
	ErrIntrinsicGas = errors.New("intrinsic gas too low")
	// ErrKnownTransaction transaction has beed added to pool
	ErrKnownTransaction = errors.New("known transaction")
)

//PoolConfig PoolConfig
type PoolConfig struct {
	PoolSize int           // Maximum number of executable transaction slots for all accounts
	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

//DefaultTxPoolConfig DefaultTxPoolConfig
var defaultTxPoolConfig = PoolConfig{
	PoolSize: 20000,
	Lifetime: 300,
}

//TxPool TxPool
type TxPool struct {
	config PoolConfig
	chain  *chain.Chain
	stateC *state.Creator
	goes   co.Goes
	done   chan struct{}
	all    *cache.RandCache
	rw     sync.RWMutex
	txFeed event.Feed
	scope  event.SubscriptionScope
}

//New construct a new txpool
func New(chain *chain.Chain, stateC *state.Creator) *TxPool {
	pool := &TxPool{
		config: defaultTxPoolConfig,
		chain:  chain,
		stateC: stateC,
		done:   make(chan struct{}),
	}
	pool.all = cache.NewRandCache(pool.config.PoolSize)
	pool.goes.Go(pool.dequeue)
	return pool
}

//IsKonwnTransactionError whether err is a ErrKnownTransaction
func (pool *TxPool) IsKonwnTransactionError(err error) bool {
	return ErrKnownTransaction == err
}

//Add transaction
func (pool *TxPool) Add(tx *tx.Transaction) error {
	pool.rw.Lock()
	defer pool.rw.Unlock()

	txID := tx.ID()
	if _, ok := pool.all.Get(txID); ok {
		return ErrKnownTransaction
	}
	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx); err != nil {
		return err
	}
	if pool.all.Len() > pool.config.PoolSize {
		if picked, ok := pool.all.Pick().Value.(*txObject); ok {
			pool.all.Remove(picked.Tx().ID())
		}
	}
	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return err
	}
	sp, err := pool.shouldPending(tx, bestBlock)
	if err != nil {
		return err
	}
	if sp {
		pool.all.Set(txID, newTxObject(tx, time.Now().Unix(), Pending))
	} else {
		pool.all.Set(txID, newTxObject(tx, time.Now().Unix(), Queued))
	}
	pool.goes.Go(func() { pool.txFeed.Send(tx) })
	return nil
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan *tx.Transaction) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

func (pool *TxPool) shouldPending(tx *tx.Transaction, bestBlock *block.Block) (bool, error) {
	dependsOn := tx.DependsOn()
	if dependsOn != nil {
		if _, _, err := pool.chain.GetTransaction(*dependsOn); err != nil {
			if pool.chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
	}
	blockNum := tx.BlockRef().Number()
	if blockNum > bestBlock.Header().Number() {
		return false, nil
	}
	return true, nil
}

//Dump dump transactions by TransactionCategory
func (pool *TxPool) Dump() []*tx.Transaction {
	pool.rw.RLock()
	defer pool.rw.RUnlock()

	txs := make([]*tx.Transaction, 0)
	pool.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			tx := obj.Tx()
			if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
				pool.Remove(tx.ID())
				return true
			}
			if obj.Status() == Pending {
				txs = append(txs, tx)
			}
			return true
		}
		return false
	})
	return txs
}

//Pending return all pending txs
func (pool *TxPool) Pending() []*tx.Transaction {

	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return nil
	}
	pendingObjs := pool.pendingObjs(bestBlock)
	txs := make([]*tx.Transaction, len(pendingObjs))
	for i, obj := range pendingObjs {
		txs[i] = obj.Tx()
	}
	return txs
}

func (pool *TxPool) pendingObjs(bestBlock *block.Block) txObjects {
	pool.rw.RLock()
	defer pool.rw.RUnlock()
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return nil
	}
	baseGasPrice := builtin.Params.WithState(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
	var pendingObjs txObjects
	pool.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			tx := obj.Tx()
			if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
				pool.Remove(tx.ID())
				return true
			}
			if obj.Status() == Pending {
				overallGP := obj.Tx().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
					return traverser.Get(num).ID()
				})
				obj.SetOverallGP(overallGP)
				pendingObjs = append(pendingObjs, obj)
			}
			return true
		}
		return false
	})
	sort.Slice(pendingObjs, func(i, j int) bool {
		return pendingObjs[i].OverallGP().Cmp(pendingObjs[j].OverallGP()) > 0
	})
	return pendingObjs
}

//Remove remove transaction by txID with TransactionCategory
func (pool *TxPool) Remove(txID thor.Hash) {
	pool.rw.Lock()
	defer pool.rw.Unlock()
	pool.all.Remove(txID)
}

//dequeueTxs for dequeue transactions
func (pool *TxPool) dequeue() {
	ticker := time.NewTicker(1 * time.Second)
	var bestBlock *block.Block
	defer ticker.Stop()
	for {
		select {
		case <-pool.done:
			return
		case <-ticker.C:
			b, err := pool.chain.GetBestBlock()
			if err != nil {
				continue
			}

			if bestBlock == nil {
				bestBlock = b
			} else {
				if b.Header().ID() == bestBlock.Header().ID() {
					continue
				}
				bestBlock = b
			}
			pool.update(bestBlock)
		}
	}
}

//gather objs that need to be pending
func (pool *TxPool) queuedToPendingObjs(bestBlock *block.Block) txObjects {
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return nil
	}

	baseGasPrice := builtin.Params.WithState(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())

	pool.rw.RLock()
	defer pool.rw.RUnlock()
	//can be pendinged txObjects
	var pendingObjs txObjects
	pool.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			tx := obj.Tx()
			if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
				pool.Remove(tx.ID())
				return true
			}
			if obj.Status() == Queued {
				sp, err := pool.shouldPending(obj.Tx(), bestBlock)
				if err != nil {
					return false
				}
				if sp {
					overallGP := obj.Tx().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
						return traverser.Get(num).ID()
					})
					obj.SetOverallGP(overallGP)
					obj.SetStatus(Pending)
					pendingObjs = append(pendingObjs, obj)
				}
			}
			return true
		}
		return false
	})

	sort.Slice(pendingObjs, func(i, j int) bool {
		return pendingObjs[i].OverallGP().Cmp(pendingObjs[j].OverallGP()) > 0
	})
	return pendingObjs
}

func (pool *TxPool) update(bestBlock *block.Block) {
	pendingObjs := pool.queuedToPendingObjs(bestBlock)
	pool.rw.Lock()
	defer pool.rw.Unlock()
	for _, obj := range pendingObjs {
		pool.all.Set(obj.Tx().ID(), obj)
	}
}

//Stop stop pool loop
func (pool *TxPool) Stop() {
	close(pool.done)
	pool.goes.Wait()
}

//GetTransaction returns a transaction
func (pool *TxPool) GetTransaction(txID thor.Hash) *tx.Transaction {
	pool.rw.RLock()
	defer pool.rw.RUnlock()
	if res, ok := pool.all.Get(txID); ok {
		if obj, ok := res.(*txObject); ok {
			return obj.Tx()
		}
	}
	return nil
}

func (pool *TxPool) validateTx(tx *tx.Transaction) error {
	// go-ethereum says: Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	if tx.Size() > 32*1024 {
		return errors.New("tx too large")
	}

	// resolvedTx, err := runtime.ResolveTransaction(tx)
	// if err != nil {
	// 	return err
	// }

	// // go-ethereum says: Transactions can't be negative. This may never happen using RLP decoded
	// // transactions but may occur if you create a transaction using the RPC.
	// for _, clause := range resolvedTx.Clauses {
	// 	if clause.Value().Sign() < 0 {
	// 		return errors.New("negative clause value")
	// 	}
	// }
	return nil
}
