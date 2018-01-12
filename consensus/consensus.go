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

	preHeader, err := validate(blk, c.chain)
	if err != nil {
		return false, err
	}

	state := c.stateCreator(preHeader.StateRoot())

	if err = verify(blk, preHeader, state); err != nil {
		return false, err
	}

	return predicateTrunk(state, blk.Header(), preHeader)
}
