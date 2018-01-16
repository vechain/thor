package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
)

type blockProcessor struct {
	rt   *runtime.Runtime
	sign *cry.Signing
}

func newBlockProcessor(rt *runtime.Runtime, sign *cry.Signing) *blockProcessor {
	return &blockProcessor{
		rt:   rt,
		sign: sign}
}

// ProcessBlock can execute all transactions in a block.
func (bp *blockProcessor) Process(blk *block.Block) (uint64, error) {
	receipts, totalGasUsed, totalEnergyUsed, err := bp.processTransactions(blk.Transactions())
	if err != nil {
		return 0, err
	}

	header := blk.Header()
	if header.ReceiptsRoot() != receipts.RootHash() {
		return 0, errReceiptsRoot
	}
	if header.GasUsed() != totalGasUsed {
		return 0, errGasUsed
	}

	return totalEnergyUsed, nil
}

func (bp *blockProcessor) processTransactions(transactions tx.Transactions) (tx.Receipts, uint64, uint64, error) {
	length := len(transactions)
	if length == 0 {
		return nil, 0, 0, nil
	}

	receipt, _, err := bp.rt.ExecuteTransaction(transactions[0], bp.sign)
	if err != nil {
		return nil, 0, 0, err
	}
	energyUsed := receipt.GasUsed * transactions[0].GasPrice().Uint64()

	receipts, totalGasUsed, totalEnergyUsed, err := bp.processTransactions(transactions[1:length])
	if err != nil {
		return nil, 0, 0, err
	}

	return append(receipts, receipt), totalGasUsed + receipt.GasUsed, totalEnergyUsed + energyUsed, nil
}

func checkState(state *state.State, header *block.Header) error {
	if stateRoot, err := state.Stage().Hash(); err == nil {
		if header.StateRoot() != stateRoot {
			return errStateRoot
		}
	} else {
		return err
	}
	return nil
}
