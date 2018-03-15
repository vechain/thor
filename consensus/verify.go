package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
)

func (c *Consensus) verify(blk *block.Block, parentHeader *block.Header, st *state.State) error {
	traverser := c.chain.NewTraverser(parentHeader.ID())

	if err := c.processBlock(blk, traverser, st); err != nil {
		return err
	}

	if err := traverser.Error(); err != nil {
		return err
	}

	stateRoot, err := st.Stage().Hash()
	if err != nil {
		return err
	}

	if blk.Header().StateRoot() != stateRoot {
		return errStateRoot
	}

	return nil
}

func (c *Consensus) processBlock(blk *block.Block, traverser *chain.Traverser, state *state.State) error {
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

	switch {
	case header.ReceiptsRoot() != receipts.RootHash():
		return errReceiptsRoot
	case header.GasUsed() != totalGasUsed:
		return errGasUsed
	default:
		return nil
	}
}
