package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain        chainReader
	stateCreator func(thor.Hash) *state.State
}

// New is Consensus factory.
func New(chain chainReader, stateCreator func(thor.Hash) *state.State) *Consensus {
	return &Consensus{
		chain:        chain,
		stateCreator: stateCreator}
}

// Consent is Consensus's main func.
func (c *Consensus) Consent(blk *block.Block) (isTrunk bool, err error) {
	if blk == nil {
		return false, errors.New("parameter is nil, must be *block.Block")
	}

	preHeader, err := c.getParentHeader(blk)
	if err != nil {
		return false, err
	}

	if err = validate(preHeader, blk); err != nil {
		return false, err
	}

	state := c.stateCreator(preHeader.StateRoot())

	if err = verify(state, blk); err != nil {
		return false, err
	}

	return PredicateTrunk(state, blk.Header(), preHeader)
}

func (c *Consensus) getParentHeader(blk *block.Block) (*block.Header, error) {
	preHeader, err := c.chain.GetBlockHeader(blk.ParentHash())
	if err != nil {
		if c.chain.IsNotFound(err) {
			return nil, errParentNotFound
		}
		return nil, err
	}
	return preHeader, nil
}
