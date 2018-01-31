package txpool

import (
	"errors"
	"fmt"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
	"sort"
	"sync"
	"time"
)

var (
	//ErrUnderpriced transaction underpriced
	ErrUnderpriced = errors.New("transaction underpriced")
	// ErrIntrinsicGas intrinsic gas too low
	ErrIntrinsicGas = errors.New("intrinsic gas too low")
)

//PoolConfig PoolConfig
type PoolConfig struct {
	PriceLimit uint64        // Minimum gas price to enforce for acceptance into the pool
	PoolSize   uint64        // Maximum number of executable transaction slots for all accounts
	Lifetime   time.Duration // Maximum amount of time non-executable transaction are queued
}

//DefaultTxPoolConfig DefaultTxPoolConfig
var DefaultTxPoolConfig = PoolConfig{
	PriceLimit: 1000,
	PoolSize:   1000,
	Lifetime:   200,
}

//TxPool TxPool
type TxPool struct {
	config   PoolConfig
	chain    *chain.Chain
	iterator *Iterator
	all      map[thor.Hash]*TxObject
	m        *sync.RWMutex
}

//NewTxPool NewTxPool
func NewTxPool(chain *chain.Chain) *TxPool {
	pool := &TxPool{
		config: DefaultTxPoolConfig,
		chain:  chain,
		all:    make(map[thor.Hash]*TxObject),
		m:      new(sync.RWMutex),
	}
	//config santize
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

	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return err
	}

	delay, err := packer.MeasureTxDelay(tx.BlockRef(), bestBlock.Header().ID(), pool.chain)
	conversionEn := thor.ProvedWorkToEnergy(tx.ProvedWork(), bestBlock.Header().Number(), delay)

	obj := NewTxObject(tx, conversionEn, time.Now().Unix())
	pool.all[txID] = obj
	return nil
}

//NewIterator Create Iterator for pool
func (pool *TxPool) NewIterator() *Iterator {
	pool.m.RLock()
	defer pool.m.RUnlock()

	var objs TxObjects
	for key, obj := range pool.all {
		if time.Now().Unix()-obj.CreateTime() > int64(pool.config.Lifetime) {
			delete(pool.all, key)
			continue
		}
		objs = append(objs, obj)
	}
	sort.Sort(objs)
	return newIterator(objs, pool)
}

//GetTxObject returns a txobj
func (pool *TxPool) GetTxObject(objID thor.Hash) *TxObject {
	return pool.all[objID]
}

//Delete delete a transaction
func (pool *TxPool) Delete(objID thor.Hash) {
	pool.m.Lock()
	defer pool.m.Unlock()
	if _, ok := pool.all[objID]; ok {
		delete(pool.all, objID)
	}
}

func (pool *TxPool) validateTx(tx *tx.Transaction) error {
	if _, err := tx.Signer(); err != nil {
		return err
	}
	if big.NewInt(int64(pool.config.PriceLimit)).Cmp(tx.GasPrice()) > 0 {
		return ErrUnderpriced
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
