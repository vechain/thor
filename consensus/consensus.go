package consensus

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/state"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain    chainReader
	getState func(cry.Hash) state.State
}

// New is Consensus factory.
func New(chain chainReader, getState func(cry.Hash) state.State) *Consensus {
	return &Consensus{
		chain:    chain,
		getState: getState}
}

// Consent is Consensus's main func.
func (c *Consensus) Consent(blk *block.Block) (isTrunk bool, err error) {
	if blk == nil {
		return false, errors.New("parameter is nil, must be *block.Block")
	}

	if err := c.isNotQualified(blk); err != nil {
		return false, err
	}

	return c.predicateTrunk()
}

func (c *Consensus) isNotQualified(blk *block.Block) error {
	parentHeader, err := c.getParentHeader(blk)
	if err != nil {
		return err
	}

	if err = validate(parentHeader, blk); err != nil {
		return err
	}

	return verify(c.getState(parentHeader.StateRoot()), blk)
}

func (c *Consensus) getParentHeader(blk *block.Block) (*block.Header, error) {
	parentHeader, err := c.chain.GetBlockHeader(blk.ParentHash())
	if err != nil {
		if c.chain.IsNotFound(err) {
			return nil, errParentNotFound
		}
		return nil, err
	}
	return parentHeader, nil
}

func (c *Consensus) predicateTrunk() (bool, error) {
	return false, nil
}
