// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestFlow_Schedule_POS(t *testing.T) {
	t.Skip("not implemented")
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	// mint block 1: using PoA
	root := chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, true, root)

	// mint block 2: still using PoA - fork happens
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, true, root)

	// mint block 3: using PoS
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, false, root)
}

func TestFlow_POS_MissedSlots(t *testing.T) {
	// TODO: Uncomment bottom assertions
	t.Skip("not implemented")
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	packNext(t, chain, thor.BlockInterval)   // mint block 1
	packNext(t, chain, thor.BlockInterval)   // mint block 2
	packNext(t, chain, thor.BlockInterval*2) // mint block 3 with a gap

	//st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	//staker := builtin.Staker.Native(st)
	//validator, err := staker.Get(genesis.DevAccounts()[0].Address)
	//assert.NoError(t, err)
	//assert.Equal(t, uint64(1), validator.MissedSlots)
}

func packNext(t *testing.T, chain *testchain.Chain, interval uint64) {
	t.Skip("not implemented")
	account := genesis.DevAccounts()[0]
	p := packer.New(chain.Repo(), chain.Stater(), account.Address, &account.Address, chain.GetForkConfig())
	parent := chain.Repo().BestBlockSummary()
	flow, err := p.Schedule(parent, parent.Header.Timestamp()+interval)
	assert.NoError(t, err)

	blk, stage, receipts, err := flow.Pack(account.PrivateKey, 0, false)
	assert.NoError(t, err)
	assert.NoError(t, chain.AddBlock(blk, stage, receipts))
	best := chain.Repo().BestBlockSummary()
	assert.Equal(t, best.Header.ID(), blk.Header().ID())
}

func verifyMechanism(t *testing.T, chain *testchain.Chain, isPoA bool, root trie.Root) {
	st := chain.Stater().NewState(root)

	auth := builtin.Authority.Native(st)
	candidates, err := auth.AllCandidates()
	assert.NoError(t, err)

	staker := builtin.Staker.Native(st)
	stakers, err := staker.LeaderGroup()
	assert.NoError(t, err)

	if isPoA {
		assert.Len(t, candidates, 1)
		assert.Len(t, stakers, 0)
	} else {
		assert.Len(t, candidates, 0)
		assert.Len(t, stakers, 1)
	}
}
