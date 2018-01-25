package api

import (
	"errors"
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

//TransactionInterface for manage block with chain
type TransactionInterface struct {
	chain *chain.Chain
}

//NewTransactionInterface return a BlockMananger by chain
func NewTransactionInterface(chain *chain.Chain) *TransactionInterface {
	return &TransactionInterface{
		chain: chain,
	}
}

//GetTransactionByHash return transaction by address
func (ti *TransactionInterface) GetTransactionByHash(txHash thor.Hash) (*types.Transaction, error) {

	tx, location, err := ti.chain.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}

	t, err := types.ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}

	block, err := ti.chain.GetBlock(location.BlockID)
	if err != nil {
		return nil, err
	}

	t.BlockID = location.BlockID.String()
	t.BlockNumber = block.Number()
	t.Index = location.Index
	return t, nil
}

//GetTransactionFromBlock return transaction from block with transaction index
func (ti *TransactionInterface) GetTransactionFromBlock(blockNumber uint32, index uint64) (*types.Transaction, error) {

	block, err := ti.chain.GetBlockByNumber(blockNumber)
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
	t.BlockID = block.ID().String()
	t.BlockNumber = block.Number()
	t.Index = index
	return t, nil
}
