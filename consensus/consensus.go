// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/poa"

	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	repo                 *chain.Repository
	stater               *state.Stater
	forkConfig           thor.ForkConfig
	correctReceiptsRoots map[string]string
	candidatesCache      *simplelru.LRU
	seeder               *poa.Seeder
}

// New create a Consensus instance.
func New(repo *chain.Repository, stater *state.Stater, forkConfig thor.ForkConfig) *Consensus {
	candidatesCache, _ := simplelru.NewLRU(16, nil)
	return &Consensus{
		repo:                 repo,
		stater:               stater,
		forkConfig:           forkConfig,
		correctReceiptsRoots: thor.LoadCorrectReceiptsRoots(),
		candidatesCache:      candidatesCache,
		seeder:               poa.NewSeeder(repo),
	}
}

// Process process a block.
func (c *Consensus) Process(blk *block.Block, nowTimestamp uint64) (*state.Stage, tx.Receipts, mclock.AbsTime, error) {
	header := blk.Header()

	if _, err := c.repo.GetBlockSummary(header.ID()); err != nil {
		if !c.repo.IsNotFound(err) {
			return nil, nil, 0, err
		}
	} else {
		return nil, nil, 0, errKnownBlock
	}

	parent, err := c.repo.GetBlock(header.ParentID())
	if err != nil {
		if !c.repo.IsNotFound(err) {
			return nil, nil, 0, err
		}
		return nil, nil, 0, errParentMissing
	}

	state := c.stater.NewState(parent.Header().StateRoot())

	vip191 := c.forkConfig.VIP191
	if vip191 == 0 {
		vip191 = 1
	}
	// Before process hook of VIP-191, update builtin extension contract's code to V2
	if header.Number() == vip191 {
		if err := state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes()); err != nil {
			return nil, nil, 0, err
		}
	}

	var features tx.Features
	if header.Number() >= vip191 {
		features |= tx.DelegationFeature
	}

	if header.TxsFeatures() != features {
		return nil, nil, 0, consensusError(fmt.Sprintf("block txs features invalid: want %v, have %v", features, header.TxsFeatures()))
	}

	stage, receipts, et, err := c.validate(state, blk, parent, nowTimestamp)
	if err != nil {
		return nil, nil, 0, err
	}

	return stage, receipts, et, nil
}

func (c *Consensus) NewRuntimeForReplay(header *block.Header, skipPoA bool) (*runtime.Runtime, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, err
	}
	parent, err := c.repo.GetBlock(header.ParentID())
	if err != nil {
		if !c.repo.IsNotFound(err) {
			return nil, err
		}
		return nil, errParentMissing
	}
	state := c.stater.NewState(parent.Header().StateRoot())
	if !skipPoA {
		if _, _, _, err := c.validateProposer(header, parent, state); err != nil {
			return nil, err
		}
	}

	return runtime.New(
		c.repo.NewChain(header.ParentID()),
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
