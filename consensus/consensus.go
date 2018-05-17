// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
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

// Process process a block.
func (c *Consensus) Process(blk *block.Block, nowTimestamp uint64) (*state.Stage, tx.Receipts, error) {
	header := blk.Header()

	if _, err := c.chain.GetBlockHeader(header.ID()); err != nil {
		if !c.chain.IsNotFound(err) {
			return nil, nil, err
		}
	} else {
		return nil, nil, errKnownBlock
	}

	parentHeader, err := c.chain.GetBlockHeader(header.ParentID())
	if err != nil {
		if !c.chain.IsNotFound(err) {
			return nil, nil, err
		}
		return nil, nil, errParentMissing
	}

	state, err := c.stateCreator.NewState(parentHeader.StateRoot())
	if err != nil {
		return nil, nil, err
	}

	stage, receipts, err := c.validate(state, blk, parentHeader, nowTimestamp)
	if err != nil {
		return nil, nil, err
	}

	return stage, receipts, nil
}
