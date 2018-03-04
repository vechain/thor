package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/tx"
)

type blockProcessor struct {
	rt    *runtime.Runtime
	chain *chain.Chain
}

func newBlockProcessor(rt *runtime.Runtime, chain *chain.Chain) *blockProcessor {
	return &blockProcessor{
		rt:    rt,
		chain: chain}
}

// ProcessBlock can execute all transactions in a block.
func (bp *blockProcessor) process(blk *block.Block, preHeader *block.Header) error {

	receipts, totalGasUsed, err := bp.processTransactions(blk.Transactions(), nil, 0)
	if err != nil {
		return err
	}

	header := blk.Header()
	switch {
	case header.ReceiptsRoot() != receipts.RootHash():
		return errReceiptsRoot
	case header.GasUsed() != totalGasUsed:
		return errGasUsed
	default:
		return nil
	}
}

func (bp *blockProcessor) processTransactions(
	transactions tx.Transactions,
	receipts tx.Receipts,
	totalGasUsed uint64) (tx.Receipts, uint64, error) {

	length := len(transactions)
	if length == 0 {
		return receipts, totalGasUsed, nil
	}
	receipt, _, err := bp.rt.ExecuteTransaction(transactions[0])
	if err != nil {
		return nil, 0, err
	}

	return bp.processTransactions(transactions[1:len(transactions)],
		append(receipts, receipt),
		totalGasUsed+receipt.GasUsed)
}
