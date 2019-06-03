// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain                *chain.Chain
	stateCreator         *state.Creator
	forkConfig           thor.ForkConfig
	correctReceiptsRoots map[string]string
}

// New create a Consensus instance.
func New(chain *chain.Chain, stateCreator *state.Creator, forkConfig thor.ForkConfig) *Consensus {
	return &Consensus{
		chain:                chain,
		stateCreator:         stateCreator,
		forkConfig:           forkConfig,
		correctReceiptsRoots: thor.LoadCorrectReceiptsRoots(),
	}
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

	vip191 := c.forkConfig.VIP191
	if vip191 == 0 {
		vip191 = 1
	}
	// Before process hook of VIP-191, update builtin extension contract's code to V2
	if header.Number() == vip191 {
		state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes())
	}

	var features tx.Features
	if header.Number() >= vip191 {
		features |= tx.DelegationFeature
	}

	if header.TxsFeatures() != features {
		return nil, nil, consensusError(fmt.Sprintf("block txs features invalid: want %v, have %v", features, header.TxsFeatures()))
	}

	stage, receipts, err := c.validate(state, blk, parentHeader, nowTimestamp)
	if err != nil {
		return nil, nil, err
	}

	return stage, receipts, nil
}

func (c *Consensus) NewRuntimeForReplay(header *block.Header, skipPoA bool) (*runtime.Runtime, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, err
	}
	parentHeader, err := c.chain.GetBlockHeader(header.ParentID())
	if err != nil {
		if !c.chain.IsNotFound(err) {
			return nil, err
		}
		return nil, errParentMissing
	}
	state, err := c.stateCreator.NewState(parentHeader.StateRoot())
	if err != nil {
		return nil, err
	}
	if !skipPoA {
		if err := c.validateProposer(header, parentHeader, state); err != nil {
			return nil, err
		}
	}

	return runtime.New(
		c.chain.NewSeeker(header.ParentID()),
		state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
		},
		c.forkConfig), nil
}
