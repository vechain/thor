package consensus

import (
	"errors"

	"github.com/vechain/thor/block"
)

var (
	errKnownBlock = errors.New("block already known")
)

type Consensus struct {
	chain ChainReader
	state Stater
}

func New() *Consensus {
	return &Consensus{}
}

func (c *Consensus) Consent(blk *block.Block) (isTrunk bool, err error) {
	if err := c.validate(blk); err != nil {
		return false, err
	}
	if err := c.verify(blk); err != nil {
		return false, err
	}
	return c.predicateTrunk(blk)
}

func (c *Consensus) validate(blk *block.Block) error {
	header := blk.Header()
	if blk.Body().Txs.RootHash() != header.TxsRoot() {
		return errors.New("")
	}
	if header.GasUsed().Cmp(header.GasLimit()) > 0 {
		return errors.New("")
	}

	parentHeader, err := c.chain.GetBlockHeader(blk.ParentHash())
	if err != nil {
		if c.chain.IsNotFound(err) {
			//
		}
		return err
	}
	_ = parentHeader

	return nil
}

func (c *Consensus) verify(blk *block.Block) error {
	return nil
}

func (c *Consensus) predicateTrunk(blk *block.Block) (bool, error) {
	return false, nil
}
