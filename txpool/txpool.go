package txpool

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
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
	config PoolConfig

	chain    *chain.Chain
	stateC   *state.Creator
	all      map[thor.Hash]*TxObject
	pending  map[thor.Hash]*TxObject
	queued   map[thor.Hash]*TxObject
	loopStop chan struct{}
	wg       sync.WaitGroup
	m        sync.RWMutex
	txFeed   event.Feed
	scope    event.SubscriptionScope
}

//New construct a new txpool
func New(chain *chain.Chain, stateC *state.Creator) *TxPool {
	pool := &TxPool{
		config:   defaultTxPoolConfig,
		chain:    chain,
		stateC:   stateC,
		loopStop: make(chan struct{}, 1),
		all:      make(map[thor.Hash]*TxObject),
		pending:  make(map[thor.Hash]*TxObject),
		queued:   make(map[thor.Hash]*TxObject),
	}

	go pool.loop()
	return pool
}

//IsKonwnTransactionError whether err is a ErrKnownTransaction
func (pool *TxPool) IsKonwnTransactionError(err error) bool {
	return ErrKnownTransaction == err
}

//Add transaction
func (pool *TxPool) Add(tx *tx.Transaction) error {
	pool.m.Lock()
	defer pool.m.Unlock()
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
	obj := NewTxObject(tx, time.Now().Unix())
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
	go pool.txFeed.Send(tx)
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
	pool.m.RLock()
	defer pool.m.RUnlock()
	objs := make(map[thor.Hash]*TxObject)
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
		tx := obj.Transaction()
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
	pool.m.RLock()
	defer pool.m.RUnlock()

	objs := make(map[thor.Hash]*TxObject)
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
	var txObjs TxObjects
	for id, obj := range objs {
		if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		overallGP := obj.Transaction().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
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
		txs[i] = obj.Transaction()
	}
	return txs, nil
}

//Remove remove transaction by txID with TransactionCategory
func (pool *TxPool) Remove(txID thor.Hash) {
	pool.m.Lock()
	defer pool.m.Unlock()
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
	defer ticker.Stop()
	for {
		select {
		case <-pool.loopStop:
			return
		case <-ticker.C:
			pool.m.RLock()
			bestBlock, err := pool.chain.GetBestBlock()
			if err != nil {
				continue
			}
			st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
			if err != nil {
				continue
			}
			baseGasPrice := builtin.Params.WithState(st).Get(thor.KeyBaseGasPrice)
			traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
			var txObjs TxObjects
			for id, obj := range pool.queued {
				if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
					pool.Remove(id)
					continue
				}
				overallGP := obj.Transaction().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
					return traverser.Get(num).ID()
				})
				obj.SetOverallGP(overallGP)
				txObjs = append(txObjs, obj)
			}
			pool.m.RUnlock()
			sort.Slice(txObjs, func(i, j int) bool {
				return txObjs[i].OverallGP().Cmp(txObjs[j].OverallGP()) > 0
			})
			for _, obj := range txObjs {
				tx := obj.Transaction()
				if err := pool.Add(tx); err != nil {
					if !pool.IsKonwnTransactionError(err) {
						pool.Remove(tx.ID())
					}
					continue
				}
			}
		}
	}
}

//Stop stop pool loop
func (pool *TxPool) Stop() {
	pool.loopStop <- struct{}{}
	close(pool.loopStop)
}

//GetTransaction returns a transaction
func (pool *TxPool) GetTransaction(txID thor.Hash) *tx.Transaction {
	pool.m.RLock()
	defer pool.m.RUnlock()
	if obj, ok := pool.all[txID]; ok {
		return obj.Transaction()
	}
	return nil
}

func (pool *TxPool) validateTx(tx *tx.Transaction) error {
	if _, err := tx.Signer(); err != nil {
		return err
	}
	intrGas, err := tx.IntrinsicGas()
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return ErrIntrinsicGas
	}

	return nil
}

//OnProcessed tx has been processed
func (pool *TxPool) OnProcessed(txID thor.Hash, err error) {
	//TODO
	pool.Remove(txID)
}
