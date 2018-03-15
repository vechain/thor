package consensus

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain  *chain.Chain
	stateC *state.Creator
}

// New is Consensus factory.
func New(chain *chain.Chain, stateC *state.Creator) *Consensus {
	return &Consensus{
		chain:  chain,
		stateC: stateC}
}

// Consent is Consensus's main func.
func (c *Consensus) Consent(blk *block.Block, nowTime uint64) (isTrunk bool, err error) {
	if blk == nil {
		return false, errors.New("parameter is nil, must be *block.Block")
	}

	parentHeader, err := c.validate(blk, nowTime)
	if err != nil {
		return false, err
	}

	state, err := c.stateC.NewState(parentHeader.StateRoot())
	if err != nil {
		return false, err
	}

	if err = c.verify(blk, parentHeader, state); err != nil {
		return false, err
	}

	if isTrunk, err = c.isTrunk(blk); err != nil {
		return false, err
	}

	if _, err = state.Stage().Commit(); err != nil {
		return false, err
	}

	return isTrunk, nil
}

func (c *Consensus) verify(blk *block.Block, parentHeader *block.Header, state *state.State) error {
	header := blk.Header()

	if err := newProposerHandler(c.chain, state, header, parentHeader).handle(); err != nil {
		return err
	}

	traverser := c.chain.NewTraverser(parentHeader.ID())
	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})

	if err := newBlockProcessor(rt, c.chain).process(blk, parentHeader); err != nil {
		return err
	}
	if err := traverser.Error(); err != nil {
		return err
	}
	return checkState(state, header)
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
		if bytes.Compare(blockID[:], bestID[:]) > 0 {
			return true, nil
		}
		return false, nil
	default:
		return true, nil
	}
}
