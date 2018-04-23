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
	var totalGasUsed uint64
	txs := blk.Transactions()
	receipts := make(tx.Receipts, 0, len(txs))
	passedTxs := make(map[thor.Bytes32]struct{})
	header := blk.Header()
	traverser := c.chain.NewTraverser(blk.Header().ParentID())
	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		func(num uint32) thor.Bytes32 { return traverser.Get(num).ID() })

	for _, tx := range txs {
		// check depended tx
		if dep := tx.DependsOn(); dep != nil {
			found, isReverted, err := c.hasTx(passedTxs, header.ParentID(), *dep)
			if err != nil {
				return nil, nil, err
			}
			if !found {
				return nil, nil, errors.New("bad tx: dep not found")
			}

			reverted, err := isReverted()
			if err != nil {
				return nil, nil, err
			}
			if reverted {
				return nil, nil, errors.New("bad tx: dep reverted")
			}

			passedTxs[*dep] = struct{}{} // cache
		}

		receipt, _, _, err := rt.ExecuteTransaction(tx)
		if err != nil {
			return nil, nil, err
		}

		totalGasUsed += receipt.GasUsed
		receipts = append(receipts, receipt)
		if !receipt.Reverted {
			passedTxs[tx.ID()] = struct{}{}
		}
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
