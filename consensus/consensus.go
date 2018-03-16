package consensus

import (
	"bytes"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain        *chain.Chain
	stateCreator *state.Creator
}

// New is Consensus factory.
func New(chain *chain.Chain, stateCreator *state.Creator) *Consensus {
	return &Consensus{
		chain:        chain,
		stateCreator: stateCreator}
}

// Consent is Consensus's main func.
func (c *Consensus) Consent(blk *block.Block, nowTime uint64) (isTrunk bool, err error) {
	parentHeader, err := c.validateBlock(blk, nowTime)
	if err != nil {
		return false, err
	}

	state, err := c.stateCreator.NewState(parentHeader.StateRoot())
	if err != nil {
		return false, err
	}

	if err := c.validateProposer(blk.Header(), parentHeader, state); err != nil {
		return false, err
	}

	stage, err := c.verify(blk, parentHeader, state)
	if err != nil {
		return false, err
	}

	if isTrunk, err = c.isTrunk(blk); err != nil {
		return false, err
	}

	if _, err = stage.Commit(); err != nil {
		return false, err
	}

	return isTrunk, nil
}

func (c *Consensus) isTrunk(block *block.Block) (bool, error) {
	bestBlock, err := c.chain.GetBestBlock()

	switch {
	case err != nil:
		return false, err
	case block.Header().TotalScore() < bestBlock.Header().TotalScore():
		return false, nil
	case block.Header().TotalScore() == bestBlock.Header().TotalScore():
		blockID := block.Header().ID()
		bestID := bestBlock.Header().ID()
		if bytes.Compare(blockID[:], bestID[:]) < 0 { // id 越小, Num 越小, 那么平均 score 越大
			return true, nil
		}
		return false, nil
	default:
		return true, nil
	}
}
