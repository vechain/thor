package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
)

// ProcessBlock can execute all transactions in a block.
func ProcessBlock(rt *runtime.Runtime, blk *block.Block, sign *cry.Signing) (uint64, error) {
	receipts, totalGasUsed, totalEnergyUsed, err := processTransactions(rt, blk.Transactions(), sign)
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

func processTransactions(rt *runtime.Runtime, transactions tx.Transactions, sign *cry.Signing) (tx.Receipts, uint64, uint64, error) {
	length := len(transactions)
	if length == 0 {
		return nil, 0, 0, nil
	}

	receipt, _, err := rt.ExecuteTransaction(transactions[0], sign)
	if err != nil {
		return nil, 0, 0, err
	}
	energyUsed := receipt.GasUsed * transactions[0].GasPrice().Uint64()

	receipts, totalGasUsed, totalEnergyUsed, err := processTransactions(rt, transactions[1:length], sign)
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
