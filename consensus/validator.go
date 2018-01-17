package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type validator struct {
	block *block.Block
	chain *chain.Chain
}

func newValidator(blk *block.Block, chain *chain.Chain) *validator {
	return &validator{
		block: blk,
		chain: chain}
}

func (v *validator) validate() (*block.Header, error) {
	preHeader, err := v.chain.GetBlockHeader(v.block.ParentHash())
	if err != nil {
		if v.chain.IsNotFound(err) {
			return nil, errParentNotFound
		}
		return nil, err
	}

	header := v.block.Header()
	gasLimit := header.GasLimit()

	switch {
	case preHeader.Timestamp() >= v.block.Timestamp():
		return nil, errTimestamp
	case !thor.GasLimit(gasLimit).IsValid(preHeader.GasLimit()):
		return nil, errGasLimit
	case header.GasUsed() > gasLimit:
		return nil, errGasUsed
	case header.TxsRoot() != v.block.Body().Txs.RootHash():
		return nil, errTxsRoot
	case !v.validateTransactions(v.block.Transactions()):
		return nil, errTransaction
	default:
		return preHeader, nil
	}
}

func (v *validator) validateTransactions(transactions tx.Transactions) bool {
	length := len(transactions)
	if length == 0 {
		return true
	}
	return v.validateTransaction(transactions[0]) && v.validateTransactions(transactions[1:length])
}

func (v *validator) validateTransaction(transaction *tx.Transaction) bool {
	switch {
	case len(transaction.Clauses()) == 0:
		return false
	case transaction.TimeBarrier() > v.block.Timestamp():
		return false
	default:
		_, err := v.chain.LookupTransaction(v.block.ParentHash(), transaction.Hash())
		return v.chain.IsNotFound(err)
	}
}
