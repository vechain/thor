package txpool

import (
	"errors"
	"fmt"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
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
	config PoolConfig
	chain  *chain.Chain
	all    *txList
}

//NewTxPool NewTxPool
func NewTxPool(chain *chain.Chain) *TxPool {
	pool := &TxPool{
		config: DefaultTxPoolConfig,
		chain:  chain,
		all:    newTxList(),
	}
	//config santize
	return pool
}

//检查transaction
func (pool *TxPool) validateTx(tx *tx.Transaction) error {
	if _, err := tx.Signer(); err != nil {
		return err
	}
	if big.NewInt(int64(pool.config.PriceLimit)).Cmp(tx.GasPrice()) > 0 {
		return ErrUnderpriced
	}
	//TODO value 验证
	intrGas, err := tx.IntrinsicGas()
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return ErrIntrinsicGas
	}

	return nil
}

//Add transaction
func (pool *TxPool) Add(tx *tx.Transaction) error {
	txID := tx.ID()
	if pool.all.IsExists(tx) {
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
	pool.all.AddTxObject(obj)
	pool.all.SortByPrice()
	//交易池已满，舍弃低价交易
	if uint64(pool.all.Len()) >= pool.config.PoolSize {
		pool.all.DiscardTail(pool.all.Len() - int(pool.config.PoolSize-1))
	}
	pool.all.Reset(int64(pool.config.Lifetime))
	return nil
}

//NewIterator return an Iterator
func (pool *TxPool) NewIterator() *Iterator {
	return newIterator(pool.all)
}

//GetTxObject return txobj
func (pool *TxPool) GetTxObject(objID thor.Hash) *TxObject {
	return pool.all.GetObj(objID)
}
