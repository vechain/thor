package txpool

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"sort"
	"sync"
	"time"
)

var (
	// ErrIntrinsicGas intrinsic gas too low
	ErrIntrinsicGas = errors.New("intrinsic gas too low")
)

//TxAddedEvent TxAddedEvent
type TxAddedEvent struct{ Tx *tx.Transaction }

//PoolConfig PoolConfig
type PoolConfig struct {
	QueueLimit uint64
	PoolSize   uint64        // Maximum number of executable transaction slots for all accounts
	Lifetime   time.Duration // Maximum amount of time non-executable transaction are queued
}

//DefaultTxPoolConfig DefaultTxPoolConfig
var DefaultTxPoolConfig = PoolConfig{
	QueueLimit: 1024,
	PoolSize:   10000,
	Lifetime:   200,
}

//TxPool TxPool
type TxPool struct {
	config   PoolConfig
	iterator *Iterator
	all      map[thor.Hash]*TxObject
	m        sync.RWMutex
	txFeed   event.Feed
	scope    event.SubscriptionScope
}

//New construct a new txpool
func New() *TxPool {
	pool := &TxPool{
		config: DefaultTxPoolConfig,
		all:    make(map[thor.Hash]*TxObject),
	}
	return pool
}

//Add transaction
func (pool *TxPool) Add(tx *tx.Transaction) error {
	pool.m.Lock()
	defer pool.m.Unlock()

	txID := tx.ID()
	if _, ok := pool.all[txID]; ok {
		return fmt.Errorf("known transaction: %x", txID)
	}

	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx); err != nil {
		return err
	}
	pool.all[txID] = NewTxObject(tx, time.Now().Unix())
	go pool.txFeed.Send(TxAddedEvent{tx})
	return nil
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan<- TxAddedEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

//NewIterator Create Iterator for pool
func (pool *TxPool) NewIterator(chain *chain.Chain, stateC *state.Creator) (*Iterator, error) {
	pool.m.RLock()
	defer pool.m.RUnlock()

	bestBlock, err := chain.GetBestBlock()
	if err != nil {
		return nil, err
	}
	st, err := stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return nil, err
	}

	baseGasPrice := builtin.Params.Get(st, thor.KeyBaseGasPrice)
	traverser := chain.NewTraverser(bestBlock.Header().ID())
	var objs TxObjects
	l := uint64(len(pool.all))
	i := uint64(0)
	for key, obj := range pool.all {
		if time.Now().Unix()-obj.CreationTime() > int64(pool.config.Lifetime) {
			pool.Remove(key)
			continue
		}
		overallGP := obj.Transaction().OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})
		obj.SetOverallGP(overallGP)
		if l <= pool.config.QueueLimit {
			objs = append(objs, obj)
		} else if i < pool.config.QueueLimit {
			objs = append(objs, obj)
			i++
		} else {
			break
		}
	}

	sort.Slice(objs, func(i, j int) bool {
		return objs[i].OverallGP().Cmp(objs[j].OverallGP()) > 0
	})

	return newIterator(objs, pool), nil
}

//Remove remove a transaction
func (pool *TxPool) Remove(objID thor.Hash) {
	pool.m.Lock()
	defer pool.m.Unlock()
	if _, ok := pool.all[objID]; ok {
		delete(pool.all, objID)
	}
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
