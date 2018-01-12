package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/tx"
)

func validate(blk *block.Block, chain chainReader) (*block.Header, error) {
	preHeader, err := chain.GetBlockHeader(blk.ParentHash())
	if err != nil {
		if chain.IsNotFound(err) {
			return nil, errParentNotFound
		}
		return nil, err
	}

	if preHeader.Timestamp() >= blk.Timestamp() {
		return nil, errTimestamp
	}

	header := blk.Header()

	if header.TxsRoot() != blk.Body().Txs.RootHash() {
		return nil, errTxsRoot
	}

	if header.GasUsed() > header.GasLimit() {
		return nil, errGasUsed
	}

	for _, transaction := range blk.Transactions() {
		if !validateTransaction(transaction, blk, chain) {
			return nil, errTransaction
		}
	}

	return preHeader, nil
}

func validateTransaction(transaction *tx.Transaction, blk *block.Block, chain chainReader) bool {
	if len(transaction.Clauses()) == 0 {
		return false
	}

	if transaction.TimeBarrier() > blk.Timestamp() {
		return false
	}

	if _, err := chain.LookupTransaction(blk.ParentHash(), transaction.Hash()); chain.IsNotFound(err) {
		return true
	}

	return false
}
