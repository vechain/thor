// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"errors"
	"fmt"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/poa"

	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	repo                 *chain.Repository
	stater               *state.Stater
	seeder               *poa.Seeder
	forkConfig           thor.ForkConfig
	correctReceiptsRoots map[string]string
	candidatesCache      *simplelru.LRU
}

// New create a Consensus instance.
func New(repo *chain.Repository, stater *state.Stater, forkConfig thor.ForkConfig) *Consensus {
	candidatesCache, _ := simplelru.NewLRU(16, nil)
	return &Consensus{
		repo:                 repo,
		stater:               stater,
		seeder:               poa.NewSeeder(repo),
		forkConfig:           forkConfig,
		correctReceiptsRoots: thor.LoadCorrectReceiptsRoots(),
		candidatesCache:      candidatesCache,
	}
}

// Process process a block.
func (c *Consensus) Process(parentSummary *chain.BlockSummary, blk *block.Block, nowTimestamp uint64, blockConflicts uint32) (*state.Stage, tx.Receipts, error) {
	header := blk.Header()
	state := c.stater.NewState(parentSummary.Root())
	fmt.Println("Consensus StateRoot: ", parentSummary.Root().Hash.String())

	var features tx.Features
	if header.Number() >= c.forkConfig.VIP191 {
		features |= tx.DelegationFeature
	}

	if header.TxsFeatures() != features {
		return nil, nil, consensusError(fmt.Sprintf("block txs features invalid: want %v, have %v", features, header.TxsFeatures()))
	}

	stage, receipts, err := c.validate(state, blk, parentSummary.Header, nowTimestamp, blockConflicts)
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
	parentSummary, err := c.repo.GetBlockSummary(header.ParentID())
	if err != nil {
		if !c.repo.IsNotFound(err) {
			return nil, err
		}
		return nil, errors.New("parent block is missing")
	}
	state := c.stater.NewState(parentSummary.Root())
	if !skipPoA {
		if _, err := c.validateProposer(header, parentSummary.Header, state); err != nil {
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
