package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/tx"
)

type blockProcessor struct {
	rt *runtime.Runtime
}

func newBlockProcessor(rt *runtime.Runtime) *blockProcessor {
	return &blockProcessor{
		rt: rt}
}

// ProcessBlock can execute all transactions in a block.
func (bp *blockProcessor) process(blk *block.Block) (uint64, error) {
	receipts, totalGasUsed, totalEnergyUsed, err := bp.processTransactions(blk.Transactions(), nil, 0, 0)
	if err != nil {
		return 0, err
	}

	header := blk.Header()
	switch {
	case header.ReceiptsRoot() != receipts.RootHash():
		return 0, errReceiptsRoot
	case header.GasUsed() != totalGasUsed:
		return 0, errGasUsed
	default:
		return totalEnergyUsed, nil
	}
}

func (bp *blockProcessor) processTransactions(
	transactions tx.Transactions,
	receipts tx.Receipts,
	totalGasUsed uint64,
	totalEnergyUsed uint64) (tx.Receipts, uint64, uint64, error) {

	length := len(transactions)
	if length == 0 {
		return receipts, totalGasUsed, totalEnergyUsed, nil
	}

	receipt, _, err := bp.rt.ExecuteTransaction(transactions[0])
	if err != nil {
		return nil, 0, 0, err
	}

	return bp.processTransactions(transactions[1:len(transactions)],
		append(receipts, receipt),
		totalGasUsed+receipt.GasUsed,
		totalEnergyUsed+receipt.GasUsed*transactions[0].GasPrice().Uint64())
}
