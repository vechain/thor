package api

import (
	"errors"

	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/chain/persist"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type transactionGetter interface {
	blockGetter
	GetTransaction(thor.Hash) (*tx.Transaction, *persist.TxLocation, error)
}

//TransactionInterface for manage block with chain
type TransactionInterface struct {
	txGetter transactionGetter
}

//NewTransactionInterface return a BlockMananger by chain
func NewTransactionInterface(txGetter transactionGetter) *TransactionInterface {
	return &TransactionInterface{
		txGetter: txGetter,
	}
}

//GetTransactionByHash return transaction by address
func (ti *TransactionInterface) GetTransactionByHash(txHash thor.Hash) (*types.Transaction, error) {

	tx, location, err := ti.txGetter.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}

	t, err := types.ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}

	block, err := ti.txGetter.GetBlock(location.BlockID)
	if err != nil {
		return nil, err
	}

	t.BlockID = location.BlockID.String()
	t.BlockNumber = block.Header().Number()
	t.Index = location.Index
	return t, nil
}

//GetTransactionFromBlock return transaction from block with transaction index
func (ti *TransactionInterface) GetTransactionFromBlock(blockNumber uint32, index uint64) (*types.Transaction, error) {

	block, err := ti.txGetter.GetBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}

	txs := block.Transactions()
	if int(index+1) > len(txs) {
		return nil, errors.New(" Transaction not found! ")
	}

	tx := txs[index]
	t, err := types.ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}

	header := block.Header()
	t.BlockID = header.ID().String()
	t.BlockNumber = header.Number()
	t.Index = index
	return t, nil
}
