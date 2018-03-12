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

	preHeader, err := newValidator(blk, c.chain).validate(nowTime)
	if err != nil {
		return false, err
	}

	state, err := c.stateC.NewState(preHeader.StateRoot())
	if err != nil {
		return false, err
	}

	if err = c.verify(blk, preHeader, state); err != nil {
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

func (c *Consensus) verify(blk *block.Block, preHeader *block.Header, state *state.State) error {
	header := blk.Header()

	if err := newProposerHandler(c.chain, state, header, preHeader).handle(); err != nil {
		return err
	}

	traverser := c.chain.NewTraverser(preHeader.ID())
	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		func(num uint32) thor.Hash {
			return traverser.Get(num).ID()
		})

	if err := newBlockProcessor(rt, c.chain).process(blk, preHeader); err != nil {
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
		if bytes.Compare(blockID[:], bestID[:]) < 0 { // id 越小, Num 越小, 那么平均 score 越大
			return true, nil
		}
		return false, nil
	default:
		return true, nil
	}
}
