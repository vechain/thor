package consensus

import (
	"bytes"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
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
func (c *Consensus) Consent(blk *block.Block, nowTimestamp uint64) (isTrunk bool, receipts tx.Receipts, err error) {
	header := blk.Header()

	if _, err := c.chain.GetBlockHeader(header.ID()); err != nil {
		if !c.chain.IsNotFound(err) {
			return false, nil, err
		}
	} else {
		return false, nil, errKnownBlock
	}

	parent, err := c.chain.GetBlockHeader(header.ParentID())
	if err != nil {
		if !c.chain.IsNotFound(err) {
			return false, nil, err
		}
		return false, nil, errParentNotFound
	}

	if err := c.validateBlockHeader(header, parent, nowTimestamp); err != nil {
		return false, nil, err
	}

	state, err := c.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return false, nil, err
	}

	if err := c.validateProposer(header, parent, state); err != nil {
		return false, nil, err
	}

	if err := c.validateBlockBody(blk); err != nil {
		return false, nil, err
	}

	stage, receipts, err := c.verifyBlock(blk, state)
	if err != nil {
		return false, nil, err
	}

	if isTrunk, err = c.IsTrunk(header); err != nil {
		return false, nil, err
	}

	if _, err = stage.Commit(); err != nil {
		return false, nil, err
	}

	return isTrunk, receipts, nil
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
