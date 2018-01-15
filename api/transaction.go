package api

import (
	"errors"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
)

//Transaction transaction
type Transaction struct {
	Hash        thor.Hash
	GasPrice    *big.Int
	Gas         uint64
	TimeBarrier uint64
	From        thor.Address

	Clauses tx.Clauses
}

//Transactions transactions
type Transactions []*Transaction

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
func (ti *TransactionInterface) GetTransactionByHash(txHash thor.Hash) (*Transaction, error) {
	tx, location, err := ti.chain.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}
	signing := cry.NewSigning(location.BlockHash)
	return convertTransactionWithSigning(tx, signing), nil

}

//GetTransactionFromBlock return transaction from block with transaction index
func (ti *TransactionInterface) GetTransactionFromBlock(blockNumber uint32, index uint64) (*Transaction, error) {
	block, err := ti.chain.GetBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}
	txs := block.Transactions()
	if int(index+1) > len(txs) {
		return nil, errors.New(" Transaction not found! ")
	}
	tx := txs[index]
	signing := cry.NewSigning(block.Hash())
	return convertTransactionWithSigning(tx, signing), nil
}

func convertTransactionWithSigning(tx *tx.Transaction, signing *cry.Signing) *Transaction {
	from, err := signing.Signer(tx)
	if err != nil {
		return nil
	}
	return &Transaction{
		From:        from,
		Hash:        tx.SigningHash(),
		Clauses:     tx.Clauses(),
		GasPrice:    tx.GasPrice(),
		Gas:         tx.Gas(),
		TimeBarrier: tx.TimeBarrier(),
	}
}
