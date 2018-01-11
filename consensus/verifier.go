package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func verify(state *state.State, blk *block.Block) error {
	header := blk.Header()

	receiptsRoot, gasUsed, err := ProcessBlock(state, blk)
	if err != nil {
		return err
	}

	if stageRoot, err := state.Stage().Hash(); err == nil {
		if header.StateRoot() != stageRoot {
			return errStateRoot
		}
	} else {
		return err
	}

	if header.GasUsed() != gasUsed {
		return errGasUsed
	}

	if header.ReceiptsRoot() != receiptsRoot {
		return errReceiptsRoot
	}

	return nil
}

func getHash(uint64) thor.Hash {
	return thor.Hash{}
}

// ProcessBlock can execute all transactions in a block.
func ProcessBlock(state *state.State, blk *block.Block) (thor.Hash, uint64, error) {
	rt := runtime.New(state, blk.Header(), getHash)
	receipts, totalGasUsed, err := processTransactions(rt, blk.Transactions())
	if err != nil {
		return thor.Hash{}, 0, err
	}
	return receipts.RootHash(), totalGasUsed, nil
}

func processTransactions(rt *runtime.Runtime, transactions tx.Transactions) (tx.Receipts, uint64, error) {
	length := len(transactions)
	if length == 0 {
		return nil, 0, nil
	}

	receipt, _, err := rt.ExecuteTransaction(transactions[0])
	if err != nil {
		return nil, 0, err
	}

	receipts, totalGasUsed, err := processTransactions(rt, transactions[1:length])
	if err != nil {
		return nil, 0, err
	}

	return append(receipts, receipt), totalGasUsed + receipt.GasUsed, nil
}
