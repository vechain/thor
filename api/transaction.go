package api

import (
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type transactionPool interface {
	Add(tx *tx.Transaction) error
	GetTransaction(txID thor.Hash) *tx.Transaction
}

//TransactionInterface for manage block with chain
type TransactionInterface struct {
	chain  *chain.Chain
	txPool *txpool.TxPool
}

//NewTransactionInterface return a BlockMananger by chain
func NewTransactionInterface(chain *chain.Chain, txPool *txpool.TxPool) *TransactionInterface {
	return &TransactionInterface{
		chain:  chain,
		txPool: txPool,
	}
}

//GetTransactionByID return transaction by transaction id
func (ti *TransactionInterface) GetTransactionByID(txID thor.Hash) (*types.Transaction, error) {

	if pengdingTransaction := ti.txPool.GetTransaction(txID); pengdingTransaction != nil {
		return types.ConvertTransaction(pengdingTransaction)
	}
	tx, location, err := ti.chain.GetTransaction(txID)
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
	t.BlockNumber = block.Header().Number()
	t.Index = location.Index
	return t, nil
}

//SendTransaction send a transactoion
func (ti *TransactionInterface) SendTransaction(raw *types.RawTransaction) error {
	bestblk, err := ti.chain.GetBestBlock()
	if err != nil {
		return err
	}
	builder, err := types.BuilcRawTransaction(raw)
	if err != nil {
		return err
	}
	builder.ChainTag(bestblk.Header().ChainTag())
	transaction := builder.Build().WithSignature(raw.Sig)
	return ti.txPool.Add(transaction)

}
