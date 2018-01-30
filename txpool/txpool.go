package txpool

import (
	"errors"
	"fmt"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/tx"
	"math/big"
	"time"
)

var (
	//ErrOversizedData ErrOversizedData
	ErrOversizedData = errors.New("too large data")
	//ErrNegativeValue ErrNegativeValue
	ErrNegativeValue = errors.New("neg value")
	//ErrInsufficientFunds ErrInsufficientFunds
	ErrInsufficientFunds = errors.New("ErrInsufficientFunds")
	//ErrReplaceUnderpriced ErrReplaceUnderpriced
	ErrReplaceUnderpriced = errors.New("ErrReplaceUnderpriced")
	//ErrGasLimit ErrGasLimit
	ErrGasLimit = errors.New("exceeds block gas limit")
	//ErrUnderpriced ErrUnderpriced
	ErrUnderpriced = errors.New("transaction underpriced")
	// ErrIntrinsicGas is returned if the transaction is specified to use less gas
	// than required to start the invocation.
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
	//交易池已满，舍弃低价交易
	if uint64(pool.all.Len()) >= pool.config.PoolSize {
		pool.all.SortByPrice()
		cheapestTx := pool.all.Index(0)
		//价格比较准则需更改
		if cheapestTx.GasPrice().Cmp(tx.GasPrice()) >= 0 {
			return ErrUnderpriced
		}
		pool.all.DiscardTail(pool.all.Len() - int(pool.config.PoolSize-1))
	}
	pool.all.AddTransaction(tx, 20)
	return nil
}
