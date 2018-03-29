package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func (c *Consensus) verifyBlock(blk *block.Block, state *state.State) (*state.Stage, tx.Receipts, error) {

	header := blk.Header()
	traverser := c.chain.NewTraverser(blk.Header().ParentID())
	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		func(num uint32) thor.Hash { return traverser.Get(num).ID() })
	var (
		txs          = blk.Transactions()
		totalGasUsed uint64
		receipts     = make(tx.Receipts, 0, len(txs))
	)

	for _, tx := range txs {
		receipt, _, _, err := rt.ExecuteTransaction(tx)
		if err != nil {
			return nil, nil, err
		}
		totalGasUsed += receipt.GasUsed
		receipts = append(receipts, receipt)
	}

	if header.GasUsed() != totalGasUsed {
		return nil, nil, errors.New("incorrect block gas used")
	}
	if header.ReceiptsRoot() != receipts.RootHash() {
		return nil, nil, errors.New("incorrect block receipts root")
	}

	if err := traverser.Error(); err != nil {
		return nil, nil, err
	}

	stage := state.Stage()
	root, err := stage.Hash()
	if err != nil {
		return nil, nil, err
	}

	if blk.Header().StateRoot() != root {
		return nil, nil, errors.New("incorrect block state root")
	}

	return stage, receipts, nil
}
