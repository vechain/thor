package consensus

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain        *chain.Chain
	stateCreator *state.Creator
}

// New create a Consensus instance.
func New(chain *chain.Chain, stateCreator *state.Creator) *Consensus {
	return &Consensus{
		chain:        chain,
		stateCreator: stateCreator}
}

// Consent is Consensus's main func.
func (c *Consensus) Consent(blk *block.Block, nowTimestamp uint64) (bool, tx.Receipts, [][]tx.TransferLogs, error) {
	header := blk.Header()

	if _, err := c.chain.GetBlockHeader(header.ID()); err != nil {
		if !c.chain.IsNotFound(err) {
			return false, nil, nil, err
		}
	} else {
		return false, nil, nil, errKnownBlock
	}

	parent, err := c.chain.GetBlockHeader(header.ParentID())
	if err != nil {
		if !c.chain.IsNotFound(err) {
			return false, nil, nil, err
		}
		return false, nil, nil, errParentNotFound
	}

	if err := c.validateBlockHeader(header, parent, nowTimestamp); err != nil {
		return false, nil, nil, err
	}

	state, err := c.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return false, nil, nil, err
	}

	if err := c.validateProposer(header, parent, state); err != nil {
		return false, nil, nil, err
	}

	if err := c.validateBlockBody(blk); err != nil {
		return false, nil, nil, err
	}

	stage, receipts, transferLogs, err := c.verifyBlock(blk, state)
	if err != nil {
		return false, nil, nil, err
	}

	isTrunk, err := c.IsTrunk(header)
	if err != nil {
		return false, nil, nil, err
	}

	if _, err = stage.Commit(); err != nil {
		return false, nil, nil, err
	}

	return isTrunk, receipts, transferLogs, nil
}

// IsTrunk to determine if the block can be head of trunk.
func (c *Consensus) IsTrunk(header *block.Header) (bool, error) {
	bestBlock, err := c.chain.GetBestBlock()
	if err != nil {
		return false, err
	}

	if header.TotalScore() < bestBlock.Header().TotalScore() {
		return false, nil
	}

	if header.TotalScore() > bestBlock.Header().TotalScore() {
		return true, nil
	}

	// total scores are equal
	if bytes.Compare(header.ID().Bytes(), bestBlock.Header().ID().Bytes()) < 0 {
		// smaller ID is preferred, since block with smaller ID usually has larger average score.
		// also, it's a deterministic decision.
		return true, nil
	}
	return false, nil
}

// FindTransaction to get the existence of a transaction on the chain identified by parentID, and
// also give a chance to check the reverted flag in receipt if the transaction exists.
func FindTransaction(
	chain *chain.Chain,
	parentID thor.Bytes32,
	processedTxs map[thor.Bytes32]bool, // txID -> reverted
	txID thor.Bytes32,
) (found bool, isReverted func() (bool, error), err error) {

	if reverted, ok := processedTxs[txID]; ok {
		return true, func() (bool, error) {
			return reverted, nil
		}, nil
	}

	loc, err := chain.LookupTransaction(parentID, txID)
	if err != nil {
		if chain.IsNotFound(err) {
			return false, func() (bool, error) { return false, nil }, nil
		}
		return false, nil, err
	}

	return true, func() (bool, error) {
		receipts, err := chain.GetBlockReceipts(loc.BlockID)
		if err != nil {
			return false, err
		}

		if loc.Index >= uint64(len(receipts)) {
			return false, errors.New("receipt index out of range")
		}

		return receipts[loc.Index].Reverted, nil
	}, nil
}
