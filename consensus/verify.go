package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
)

func (c *Consensus) verify(blk *block.Block, parentHeader *block.Header, state *state.State) (*state.Stage, error) {
	if err := c.processBlock(blk, state); err != nil {
		return nil, err
	}

	stage := state.Stage()
	root, err := stage.Hash()
	if err != nil {
		return nil, err
	}

	if blk.Header().StateRoot() != root {
		return nil, errStateRoot
	}

	return stage, nil
}

func (c *Consensus) processBlock(blk *block.Block, state *state.State) error {
	traverser := c.chain.NewTraverser(blk.Header().ParentID())

	header := blk.Header()
	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})
	var (
		totalGasUsed uint64
		receipts     Tx.Receipts
	)

	for _, tx := range blk.Transactions() {
		receipt, _, err := rt.ExecuteTransaction(tx)
		if err != nil {
			return err
		}
		totalGasUsed += receipt.GasUsed
		receipts = append(receipts, receipt)
	}

	if header.GasUsed() != totalGasUsed {
		return errGasUsed
	}
	if header.ReceiptsRoot() != receipts.RootHash() {
		return errReceiptsRoot
	}

	return traverser.Error()
}
