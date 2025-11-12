// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus_test

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/vrf"
)

func newHayabusaSetup(t *testing.T, fork, tp uint32, forked bool) *testchain.Chain {
	thor.MockBlocklist([]string{})
	chain, err := testchain.NewWithFork(&thor.ForkConfig{
		HAYABUSA: fork,
	}, tp)
	require.NoError(t, err)

	if forked {
		for range fork {
			require.NoError(t, chain.MintBlock())
		}
		require.NoError(t, chain.AddValidators())
		for chain.Repo().BestBlockSummary().Header.Number() <= fork+tp {
			require.NoError(t, chain.MintBlock())
		}
		assertConsensus(t, chain, false, true)
	}

	return chain
}

func assertConsensus(t *testing.T, chain *testchain.Chain, isPoA, stakerDeployed bool) {
	active, err := builtin.Staker.Native(chain.State()).IsPoSActive()
	assert.NoError(t, err)
	if isPoA {
		assert.False(t, active, "consensus should be PoA")
	} else {
		assert.True(t, active, "consensus should be PoS")
	}

	code, err := chain.State().GetCode(builtin.Staker.Address)
	assert.NoError(t, err)
	if stakerDeployed {
		assert.NotEmpty(t, code, "staker contract should be deployed")
	} else {
		assert.Empty(t, code, "staker contract should not be deployed")
	}
}

// copyBlock creates a copy of blk and applies the given function to the block builder before building.
// The block does not get signed
func copyBlock(blk *block.Block, apply func(*block.Builder)) *block.Block {
	header := blk.Header()
	builder := new(block.Builder).
		ParentID(header.ParentID()).
		Timestamp(header.Timestamp()).
		TotalScore(header.TotalScore()).
		GasLimit(header.GasLimit()).
		GasUsed(header.GasUsed()).
		Beneficiary(header.Beneficiary()).
		StateRoot(header.StateRoot()).
		ReceiptsRoot(header.ReceiptsRoot()).
		TransactionFeatures(header.TxsFeatures()).
		Alpha(header.Alpha()).
		BaseFee(header.BaseFee())

	if blk.Header().COM() {
		builder.COM()
	}

	for _, tx := range blk.Transactions() {
		builder.Transaction(tx)
	}

	apply(builder)

	return builder.Build()
}

func TestConsensus_PosFork(t *testing.T) {
	chain := newHayabusaSetup(t, 2, 2, false)

	// mint block 1: update the MBP
	require.NoError(t, chain.MintBlock())
	assertConsensus(t, chain, true, false)

	// mint block 2: chain should set the staker contract, still using PoA
	require.NoError(t, chain.MintBlock())
	assertConsensus(t, chain, true, true)

	// mint block 3: add validator to the contract
	require.NoError(t, chain.AddValidators())

	// mint block 4: chain should switch to PoS
	require.NoError(t, chain.MintBlock())
	assertConsensus(t, chain, false, true)
}

func TestConsensus_PoS_IsTheTime(t *testing.T) {
	chain := newHayabusaSetup(t, 2, 2, true)

	parent := chain.Repo().BestBlockSummary()
	require.NoError(t, chain.MintBlock())
	best, err := chain.BestBlock()
	require.NoError(t, err)
	signer, err := best.Header().Signer()
	require.NoError(t, err)

	var privateKey *ecdsa.PrivateKey
	for _, acc := range genesis.DevAccounts() {
		if acc.Address != signer {
			privateKey = acc.PrivateKey
			break
		}
	}

	copied := signBlock(t, parent.Header, best, privateKey)

	_, _, err = consensus.New(chain.Repo(), chain.Stater(), chain.GetForkConfig()).
		Process(parent, copied, copied.Header().Timestamp(), 0)

	assert.ErrorContains(t, err, "pos - block timestamp unscheduled")
}

func TestConsensus_BadScore(t *testing.T) {
	chain := newHayabusaSetup(t, 2, 2, true)

	parent := chain.Repo().BestBlockSummary()
	require.NoError(t, chain.MintBlock())
	best, err := chain.BestBlock()
	require.NoError(t, err)
	signer, err := best.Header().Signer()
	require.NoError(t, err)

	var privateKey *ecdsa.PrivateKey
	for _, acc := range genesis.DevAccounts() {
		if acc.Address == signer {
			privateKey = acc.PrivateKey
			break
		}
	}

	copied := copyBlock(best, func(b *block.Builder) {
		b.TotalScore(best.Header().TotalScore() + 10)
	})
	copied = signBlock(t, parent.Header, copied, privateKey)

	_, _, err = consensus.New(chain.Repo(), chain.Stater(), chain.GetForkConfig()).
		Process(parent, copied, copied.Header().Timestamp(), 0)

	assert.ErrorContains(t, err, "pos - block total score invalid")
}

func TestConsensus_BadBeneficiary(t *testing.T) {
	chain := newHayabusaSetup(t, 2, 2, true)

	validator := genesis.DevAccounts()[0]

	require.NoError(t, chain.MintFromABI(
		validator,
		builtin.Staker.Address,
		builtin.Staker.ABI,
		big.NewInt(0),
		"setBeneficiary",
		validator.Address,
		datagen.RandAddress(),
	))

	parent := chain.Repo().BestBlockSummary()
	best, err := chain.BestBlock()
	require.NoError(t, err)
	signer, err := best.Header().Signer()
	require.NoError(t, err)

	for signer != validator.Address {
		require.NoError(t, chain.MintBlock())
		best, err = chain.BestBlock()
		require.NoError(t, err)
		signer, err = best.Header().Signer()
		require.NoError(t, err)
	}

	copied := copyBlock(best, func(b *block.Builder) {
		b.Beneficiary(datagen.RandAddress())
	})
	copied = signBlock(t, parent.Header, copied, validator.PrivateKey)

	_, _, err = consensus.New(chain.Repo(), chain.Stater(), chain.GetForkConfig()).
		Process(parent, copied, copied.Header().Timestamp(), 0)

	assert.ErrorContains(t, err, "pos - stake beneficiary mismatch")
}

func TestConsensus_Updates(t *testing.T) {
	chain := newHayabusaSetup(t, 2, 2, true)

	next, ok := chain.NextValidator()
	require.True(t, ok, "no next validator found")

	val, err := builtin.Staker.Native(chain.State()).GetValidation(next.Address)
	require.NoError(t, err)
	require.True(t, val.IsOnline())

	chain.RemoveValidator(next.Address)
	require.NoError(t, chain.MintBlock())
	chain.AddValidator(next)

	val, err = builtin.Staker.Native(chain.State()).GetValidation(next.Address)
	require.NoError(t, err)
	require.False(t, val.IsOnline())
}

func TestConsensus_TransitionPeriodBalanceCheck(t *testing.T) {
	thor.MockBlocklist([]string{})
	fc := &thor.ForkConfig{
		HAYABUSA: 2,
	}
	// forks but never transitions to PoS within the test
	gene, err := testchain.CreateGenesis(fc, 2, 2, 100000)
	require.NoError(t, err)
	chain, err := testchain.NewIntegrationTestChainWithGenesis(gene, fc, 2)
	require.NoError(t, err)

	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())
	assertConsensus(t, chain, true, true)

	validator := genesis.DevAccounts()[0]

	require.NoError(t, chain.MintFromABI(
		validator,
		builtin.Staker.Address,
		builtin.Staker.ABI,
		staker.MinStake,
		"addValidation",
		validator.Address,
		thor.LowStakingPeriod(),
	))

	for range 32 {
		require.NoError(t, chain.MintBlock())
		best := chain.Repo().BestBlockSummary()
		signer, err := best.Header.Signer()
		require.NoError(t, err)
		if signer == validator.Address {
			t.Log("test passed, transition period is okay")
			return
		}
	}

	t.Fatal("validator failed to sign block during transition period")
}

func signBlock(t *testing.T, parent *block.Header, blk *block.Block, privateKey *ecdsa.PrivateKey) *block.Block {
	parentBeta, err := parent.Beta()
	require.NoError(t, err)

	var alpha []byte
	// initial value of chained VRF
	if len(parentBeta) == 0 {
		alpha = parent.StateRoot().Bytes()
	} else {
		alpha = parentBeta
	}

	ec, err := crypto.Sign(blk.Header().SigningHash().Bytes(), privateKey)
	require.NoError(t, err)

	_, proof, err := vrf.Prove(privateKey, alpha)
	require.NoError(t, err)
	sig, err := block.NewComplexSignature(ec, proof)
	require.NoError(t, err)

	return blk.WithSignature(sig)
}
