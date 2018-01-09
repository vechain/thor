package consensus

import (
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
)

func verify(state *state.State, blk *block.Block) error {
	header := blk.Header()

	receiptsRoot, gasUsed, err := ProcessBlock(blk)
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

	if header.GasUsed().ToBig() != gasUsed {
		return errGasUsed
	}

	if header.ReceiptsRoot() != receiptsRoot {
		return errReceiptsRoot
	}

	return nil
}

// ProcessBlock can execute all transactions in a block.
func ProcessBlock(blk *block.Block) (cry.Hash, *big.Int, error) {
	stub := &processStub{}
	receipts, totalGasUsed, err := processTransactions(stub, blk.Transactions())
	if err != nil {
		return cry.Hash{}, nil, err
	}
	return receipts.RootHash(), totalGasUsed, nil
}

func processTransactions(stub processorStub, transactions tx.Transactions) (tx.Receipts, *big.Int, error) {
	length := len(transactions)
	if length == 0 {
		return tx.Receipts{}, new(big.Int), nil
	}

	receipts, totalGasUsed, err := processTransactions(stub, transactions[:length-1])
	if err != nil {
		return nil, nil, err
	}

	receipt, gasUsed, err := stub.processTransaction(transactions[length-1])
	if err != nil {
		return nil, nil, err
	}

	return append(receipts, receipt), new(big.Int).Add(totalGasUsed, gasUsed), nil
}
