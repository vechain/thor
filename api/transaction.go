package api

import (
	"errors"
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cry"
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

	tx, _, err := ti.chain.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}
	genesisHash, err := thor.ParseHash("0x000000006d2958e8510b1503f612894e9223936f1008be2a218c310fa8c11423")
	if err != nil {
		return nil, err
	}
	signing := cry.NewSigning(genesisHash)

	return types.ConvertTransactionWithSigning(tx, signing), nil
}

//GetTransactionFromBlock return transaction from block with transaction index
func (ti *TransactionInterface) GetTransactionFromBlock(blockNumber uint32, index uint64) (*types.Transaction, error) {
	genesisHash, err := thor.ParseHash("0x000000006d2958e8510b1503f612894e9223936f1008be2a218c310fa8c11423")
	if err != nil {
		return nil, err
	}
	signing := cry.NewSigning(genesisHash)

	block, err := ti.chain.GetBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}

	txs := block.Transactions()
	if int(index+1) > len(txs) {
		return nil, errors.New(" Transaction not found! ")
	}

	tx := txs[index]

	return types.ConvertTransactionWithSigning(tx, signing), nil
}
