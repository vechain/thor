package txpool

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/vechain/thor/block"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/builtin"
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
	ErrKnownTransaction = errors.New("knwon transaction")
	// ErrPoolOverload pool size overed
	ErrPoolOverload = errors.New("transaction pool overloaded")
)

//TransactionCategory separates transaction to three categories
type TransactionCategory int

const (
	//Pending transactions
	Pending TransactionCategory = iota
	//Queued transactions
	Queued
	//All transactions
	All
)

//PoolConfig PoolConfig
type PoolConfig struct {
	PoolSize int           // Maximum number of executable transaction slots for all accounts
	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

//DefaultTxPoolConfig DefaultTxPoolConfig
var defaultTxPoolConfig = PoolConfig{
	PoolSize: 10000,
	Lifetime: 300,
}

//TxPool TxPool
type TxPool struct {
	config  PoolConfig
	chain   *chain.Chain
	stateC  *state.Creator
	goes    co.Goes
	done    chan struct{}
	all     map[thor.Hash]*txObject
	overed  map[thor.Hash]*txObject
	pending map[thor.Hash]*txObject
	queued  map[thor.Hash]*txObject
	rw      sync.RWMutex
	txFeed  event.Feed
	scope   event.SubscriptionScope
}

//New construct a new txpool
func New(chain *chain.Chain, stateC *state.Creator) *TxPool {
	pool := &TxPool{
		config:  defaultTxPoolConfig,
		chain:   chain,
		stateC:  stateC,
		done:    make(chan struct{}),
		all:     make(map[thor.Hash]*txObject),
		overed:  make(map[thor.Hash]*txObject),
		pending: make(map[thor.Hash]*txObject),
		queued:  make(map[thor.Hash]*txObject),
	}
	pool.goes.Go(pool.dequeue)
	pool.goes.Go(pool.reload)
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
	if _, ok := pool.all[txID]; ok {
		return ErrKnownTransaction
	}
	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx); err != nil {
		return err
	}
	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return err
	}
	obj := newTxObject(tx, time.Now().Unix())
	//pool overed,just set into `all` ,wait for sort all txs
	if len(pool.pending)+len(pool.queued) >= pool.config.PoolSize {
		pool.all[txID] = obj
		pool.overed[txID] = obj
		return nil
	}
	sp, err := pool.shouldPending(tx, bestBlock)
	if err != nil {
		return err
	}
	if sp {
		pool.pending[txID] = obj
	} else {
		pool.queued[txID] = obj
	}
	pool.all[txID] = obj
	pool.goes.Go(func() { pool.txFeed.Send(tx) })
	return nil
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
func (pool *TxPool) Dump(txCate TransactionCategory) []*tx.Transaction {
	pool.rw.RLock()
	defer pool.rw.RUnlock()
	objs := make(map[thor.Hash]*txObject)
	switch txCate {
	case Pending:
		objs = pool.pending
	case Queued:
		objs = pool.queued
	case All:
		objs = pool.all
	}
	txs := make([]*tx.Transaction, 0)
	for id, obj := range objs {
		tx := obj.Tx()
		if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		txs = append(txs, tx)
	}
	return txs
}

//Sorted sort transactions by OverallGasprice with TransactionCategory
func (pool *TxPool) Sorted(txCate TransactionCategory) ([]*tx.Transaction, error) {
	pool.rw.RLock()
	defer pool.rw.RUnlock()

	objs := make(map[thor.Hash]*txObject)
	switch txCate {
	case Pending:
		objs = pool.pending
	case Queued:
		objs = pool.queued
	case All:
		objs = pool.all
	}

	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return nil, err
	}
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return nil, err
	}
	baseGasPrice := builtin.Params.WithState(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
	var txObjs txObjects
	for id, obj := range objs {
		if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		overallGP := obj.Tx().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})
		obj.SetOverallGP(overallGP)
		txObjs = append(txObjs, obj)
	}

	sort.Slice(txObjs, func(i, j int) bool {
		return txObjs[i].OverallGP().Cmp(txObjs[j].OverallGP()) > 0
	})

	txs := make([]*tx.Transaction, len(objs))
	for i, obj := range txObjs {
		txs[i] = obj.Tx()
	}
	return txs, nil
}

//Remove remove transaction by txID with TransactionCategory
func (pool *TxPool) Remove(txID thor.Hash) {
	pool.rw.Lock()
	defer pool.rw.Unlock()
	if _, ok := pool.all[txID]; ok {
		delete(pool.all, txID)
	}
	if _, ok := pool.pending[txID]; ok {
		delete(pool.pending, txID)
	}
	if _, ok := pool.queued[txID]; ok {
		delete(pool.queued, txID)
	}
	if _, ok := pool.overed[txID]; ok {
		delete(pool.overed, txID)
	}
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan *tx.Transaction) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

//reload resort all txs
func (pool *TxPool) reload() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pool.done:
			return
		case <-ticker.C:
			if len(pool.overed) == 0 {
				continue
			}
			b, err := pool.chain.GetBestBlock()
			if err != nil {
				continue
			}

			pool.resetAllTxs(b)
		}
	}
}

func (pool *TxPool) allObjs(bestBlock *block.Block) txObjects {
	pool.rw.RLock()
	defer pool.rw.RUnlock()
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return nil
	}
	baseGasPrice := builtin.Params.WithState(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
	var allObjs txObjects
	for id, obj := range pool.all {
		if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		overallGP := obj.Tx().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})
		obj.SetOverallGP(overallGP)
		allObjs = append(allObjs, obj)
	}

	sort.Slice(allObjs, func(i, j int) bool {
		return allObjs[i].OverallGP().Cmp(allObjs[j].OverallGP()) > 0
	})
	return allObjs
}

func (pool *TxPool) resetAllTxs(bestBlock *block.Block) {
	allObjs := pool.allObjs(bestBlock)
	fmt.Println("allObjs", len(allObjs))
	pool.rw.Lock()
	defer pool.rw.Unlock()
	for i, obj := range allObjs {
		tx := obj.Tx()
		txID := tx.ID()
		if i <= pool.config.PoolSize {
			sp, err := pool.shouldPending(tx, bestBlock)
			if err != nil {
				return
			}
			//objs should be pending
			if sp {
				pool.pending[txID] = obj
				if _, ok := pool.overed[txID]; ok {
					delete(pool.overed, txID)
				}
				if _, ok := pool.queued[txID]; ok {
					delete(pool.queued, txID)
				}
			} else {
				//objs should be queued
				pool.queued[txID] = obj
				if _, ok := pool.overed[txID]; ok {
					delete(pool.overed, txID)
				}
				if _, ok := pool.pending[txID]; ok {
					delete(pool.pending, txID)
				}
			}
		} else {
			//objs should be overed
			pool.overed[txID] = obj
			if _, ok := pool.pending[txID]; ok {
				delete(pool.pending, txID)
			}
			if _, ok := pool.queued[txID]; ok {
				delete(pool.queued, txID)
			}
		}
	}
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
	for id, obj := range pool.queued {
		if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		sp, err := pool.shouldPending(obj.Tx(), bestBlock)
		if err != nil {
			return nil
		}
		if sp {
			overallGP := obj.Tx().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
				return traverser.Get(num).ID()
			})
			obj.SetOverallGP(overallGP)
			pendingObjs = append(pendingObjs, obj)
		}
	}

	sort.Slice(pendingObjs, func(i, j int) bool {
		return pendingObjs[i].OverallGP().Cmp(pendingObjs[j].OverallGP()) > 0
	})
	return pendingObjs
}

func (pool *TxPool) update(bestBlock *block.Block) {
	pendingObjs := pool.queuedToPendingObjs(bestBlock)
	fmt.Println("best", bestBlock)
	pool.rw.Lock()
	defer pool.rw.Unlock()
	for _, obj := range pendingObjs {
		fmt.Println("pending tx", obj.Tx())
		if len(pool.pending)+len(pool.queued) <= pool.config.PoolSize {
			txID := obj.Tx().ID()
			delete(pool.queued, txID)
			pool.pending[txID] = obj
		}
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
	if obj, ok := pool.all[txID]; ok {
		return obj.Tx()
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

//OnProcessed tx has been processed
func (pool *TxPool) OnProcessed(txID thor.Hash, err error) {
	//TODO
	pool.Remove(txID)
}
