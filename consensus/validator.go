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

func (v *validator) validate(nowTime uint64) (*block.Header, error) {
	preHeader, err := v.chain.GetBlockHeader(v.block.Header().ParentID())
	if err != nil {
		if v.chain.IsNotFound(err) {
			return nil, errParentNotFound
		}
		return nil, err
	}

	header := v.block.Header()
	gasLimit := header.GasLimit()
	transactions := v.block.Transactions()

	// Signer and IntrinsicGas will be validate in runtime.

	switch {
	case preHeader.Timestamp() >= header.Timestamp():
		return nil, errTimestamp
	case header.Timestamp() > nowTime+thor.BlockInterval:
		return nil, errDelay
	case !thor.GasLimit(gasLimit).IsValid(preHeader.GasLimit()):
		return nil, errGasLimit
	case header.GasUsed() > gasLimit:
		return nil, errGasUsed
	case header.TxsRoot() != transactions.RootHash():
		return nil, errTxsRoot
	case !v.validateTransactions(make(map[thor.Hash]bool), transactions):
		return nil, errTransaction
	default:
		return preHeader, nil
	}
}

func (v *validator) validateTransactions(validTx map[thor.Hash]bool, transactions tx.Transactions) bool {
	switch {
	case len(transactions) == 0:
		return true
	case !v.validateTransaction(validTx, transactions[0]):
		return false
	default:
		validTx[transactions[0].ID()] = true
		return v.validateTransactions(validTx, transactions[1:len(transactions)])
	}
}

func (v *validator) validateTransaction(validTx map[thor.Hash]bool, transaction *tx.Transaction) bool {
	switch {
	case len(transaction.Clauses()) == 0:
		return false
	case transaction.BlockRef().Number() >= v.block.Header().Number():
		return false
	case transaction.ChainTag() != v.block.Header().ChainTag():
		return false
	case !v.isTxDependFound(validTx, transaction):
		return false
	default:
		return v.isTxNotFound(validTx, transaction)
	}
}

func (v *validator) isTxNotFound(validTx map[thor.Hash]bool, transaction *tx.Transaction) bool {
	if _, ok := validTx[transaction.ID()]; ok { // 在当前块中找到相同交易
		return false
	}

	_, err := v.chain.LookupTransaction(v.block.Header().ParentID(), transaction.ID())
	return v.chain.IsNotFound(err)
}

func (v *validator) isTxDependFound(validTx map[thor.Hash]bool, transaction *tx.Transaction) bool {
	dependID := transaction.DependsOn()
	if dependID == nil { // 不依赖其它交易
		return true
	}

	if _, ok := validTx[*dependID]; ok { // 在当前块中找到依赖
		return true
	}

	_, err := v.chain.LookupTransaction(v.block.Header().ParentID(), *dependID)
	return err != nil // 在 chain 中找到依赖
}
