package api

import (
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

//GetTransactionByID return transaction by transaction id
func (ti *TransactionInterface) GetTransactionByID(txID thor.Hash) (*types.Transaction, error) {

	tx, location, err := ti.txGetter.GetTransaction(txID)
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
