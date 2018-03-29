package txpool

import (
	"errors"
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
		pending: make(map[thor.Hash]*txObject),
		queued:  make(map[thor.Hash]*txObject),
	}

	pool.goes.Go(pool.loop)
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
	if len(pool.all) >= pool.config.PoolSize {
		return ErrPoolOverload
	}
	txID := tx.ID()
	if _, ok := pool.all[txID]; ok {
		return ErrKnownTransaction
	}
	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx); err != nil {
		return err
	}
	obj := newTxObject(tx, time.Now().Unix())
	sp, err := pool.shouldPending(tx)
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

func (pool *TxPool) shouldPending(tx *tx.Transaction) (bool, error) {
	dependsOn := tx.DependsOn()
	if dependsOn != nil {
		if _, _, err := pool.chain.GetTransaction(*dependsOn); err != nil {
			if pool.chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
	}
	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return false, err
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
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan *tx.Transaction) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

//loop for dequeue transactions
func (pool *TxPool) loop() {
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

func (pool *TxPool) update(bestBlock *block.Block) {
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return
	}

	baseGasPrice := builtin.Params.WithState(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())

	pool.rw.RLock()
	defer pool.rw.RUnlock()

	var txObjs txObjects
	for id, obj := range pool.queued {
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
	pool.rw.RUnlock()
	sort.Slice(txObjs, func(i, j int) bool {
		return txObjs[i].OverallGP().Cmp(txObjs[j].OverallGP()) > 0
	})
	for _, obj := range txObjs {
		tx := obj.Tx()
		if err := pool.Add(tx); err != nil {
			if !pool.IsKonwnTransactionError(err) {
				pool.Remove(tx.ID())
			}
			continue
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
