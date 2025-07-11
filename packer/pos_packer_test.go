// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func TestFlow_Schedule_POS(t *testing.T) {
	config := &thor.SoloFork
	config.HAYABUSA = 2
	config.HAYABUSA_TP = 1
	config.BLOCKLIST = math.MaxUint32

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	// mint block 1: using PoA
	root := chain.Repo().BestBlockSummary().Root()
	packMbpBlock(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, true, root)

	// mint block 2: deploy staker contract, still using PoA
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, true, root)

	// mint block 3: add validator tx
	root = chain.Repo().BestBlockSummary().Root()
	packAddValidatorBlock(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, true, root)

	// mint block 4: should switch to PoS
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, true, root)

	// mint block 5: full PoS
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval)
	verifyMechanism(t, chain, false, root)

	evidence := make([]block.Header, 1)
	evidence[0] = block.Header{}
	packNextWithEvidence(t, chain, thor.BlockInterval, &evidence)
	verifyMechanism(t, chain, false, root)
	summary := chain.Repo().BestBlockSummary()
	assert.Equal(t, evidence, *summary.Header.Evidence())
}

func packNext(t *testing.T, chain *testchain.Chain, interval uint64, txs ...*tx.Transaction) {
	var evidence *[]block.Header
	packNextWithEvidence(t, chain, interval, evidence, txs...)
}

func packNextWithEvidence(t *testing.T, chain *testchain.Chain, interval uint64, evidence *[]block.Header, txs ...*tx.Transaction) {
	account := genesis.DevAccounts()[0]
	p := packer.New(chain.Repo(), chain.Stater(), account.Address, &account.Address, chain.GetForkConfig(), 0)
	parent := chain.Repo().BestBlockSummary()
	flow, _, err := p.Schedule(parent, parent.Header.Timestamp()+interval)
	assert.NoError(t, err)

	for _, trx := range txs {
		assert.NoError(t, flow.Adopt(trx))
	}

	conflicts := uint32(0)
	if evidence != nil {
		conflicts = uint32(len(*evidence))
	}

	blk, stage, receipts, err := flow.Pack(account.PrivateKey, conflicts, false, evidence)
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
		assert.Len(t, stakers, 1)
	}
}

func packMbpBlock(t *testing.T, chain *testchain.Chain, interval uint64) {
	contract := chain.Contract(builtin.Params.Address, builtin.Params.ABI, genesis.DevAccounts()[0])
	tx, err := contract.BuildTransaction("set", big.NewInt(0), thor.KeyMaxBlockProposers, big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}

	packNext(t, chain, interval, tx)
}

func packAddValidatorBlock(t *testing.T, chain *testchain.Chain, interval uint64) {
	vet := big.NewInt(25_000_000)
	vet = vet.Mul(vet, big.NewInt(1e18))

	contract := chain.Contract(builtin.Staker.Address, builtin.Staker.ABI, genesis.DevAccounts()[0])
	tx, err := contract.BuildTransaction("addValidator", vet, genesis.DevAccounts()[0].Address, uint32(360)*24*7, true)
	if err != nil {
		t.Fatal(err)
	}

	packNext(t, chain, interval, tx)
}
