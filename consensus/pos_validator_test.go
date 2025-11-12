// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

func newHayabusaSetup(t *testing.T, fork, tp uint32) *testchain.Chain {
	chain, err := testchain.NewWithFork(&thor.ForkConfig{
		HAYABUSA: fork,
	}, tp)
	require.NoError(t, err)
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

func TestConsensus_PosFork(t *testing.T) {
	thor.MockBlocklist([]string{})
	chain := newHayabusaSetup(t, 2, 2)

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

//
//	cache, err := simplelru.NewLRU(16, nil)
//	assert.NoError(t, err)
//
//	staker := builtin.Staker.Native(chain.State())
//	leaders, err := staker.LeaderGroup()
//	assert.NoError(t, err)
//	best := chain.Repo().BestBlockSummary()
//	parent, err := chain.Repo().NewBestChain().GetBlockSummary(best.Header.Number() - 1)
//	assert.NoError(t, err)
//
//	parentSig, err := parent.Header.Signer()
//	assert.NoError(t, err)
//
//	newParentHeader := new(block.Builder).
//		ParentID(parent.Header.ParentID()).
//		Timestamp(parent.Header.Timestamp()).
//		GasLimit(parent.Header.GasLimit()).
//		GasUsed(parent.Header.GasUsed()).
//		TotalScore(10003).
//		StateRoot(parent.Header.StateRoot()).
//		ReceiptsRoot(parent.Header.ReceiptsRoot()).
//		Beneficiary(parent.Header.Beneficiary()).
//		Build().Header()
//
//	_, err = setup.consensus.validateStakingProposer(best.Header, newParentHeader, builtin.Staker.Native(st))
//	assert.ErrorContains(t, err, "pos - stake beneficiary mismatch")
//
//	newLeaders = make([]validation.Leader, 0, len(leaders))
//	for _, leader := range newLeaders {
//		if leader.Address == parentSig {
//			newLeaders = append(newLeaders, validation.Leader{
//				Address:     parentSig,
//				Beneficiary: nil,
//				Endorser:    thor.Address{},
//				Weight:      10,
//				Active:      false,
//			})
//		} else {
//			newLeaders = append(newLeaders, leader)
//		}
//	}
//	cache.Add(parent.Header.ID(), newLeaders)
//	setup.consensus.validatorsCache = cache
//
//	newParentHeader = new(block.Builder).
//		ParentID(parent.Header.ParentID()).
//		Timestamp(parent.Header.Timestamp()).
//		GasLimit(parent.Header.GasLimit()).
//		GasUsed(parent.Header.GasUsed()).
//		TotalScore(1).
//		StateRoot(parent.Header.StateRoot()).
//		ReceiptsRoot(parent.Header.ReceiptsRoot()).
//		Beneficiary(parent.Header.Beneficiary()).
//		Build().Header()
//}

//func TestConsensus_POS_MissedSlots(t *testing.T) {
//	chain := newHayabusaSetup(t, 2, 2)
//	signer := genesis.DevAccounts()[0]
//
//	require.NoError(t, chain.MintBlock())
//	require.NoError(t, chain.MintBlock())
//	require.NoError(t, chain.AddValidators())
//	require.NoError(t, chain.MintBlock())
//	assertConsensus(t, chain, false, true)
//
//	best := chain.Repo().BestBlockSummary()
//	parent, err := chain.Repo().NewBestChain().GetBlockSummary(best.Header.Number() - 1)
//	assert.NoError(t, err)
//
//	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, chain.GetForkConfig(), 0)
//	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval()*2, 10_000_000)
//	assert.NoError(t, err)
//	blk, stage, receipts, err := flow.Pack(signer.PrivateKey, 0, false)
//	assert.NoError(t, err)
//	assert.NoError(t, chain.AddBlock(blk, stage, receipts))
//
//	con := consensus.New(chain.Repo(), chain.Stater(), chain.GetForkConfig())
//
//	//_, err = setup.consensus.validateStakingProposer(blk.Header(), parent.Header, builtin.Staker.Native(st))
//	//assert.NoError(t, err)
//	//staker := builtin.Staker.Native(st)
//	//validator, err := staker.GetValidation(signer.Address)
//	//assert.NoError(t, err)
//	//assert.Nil(t, validator.OfflineBlock)
//}

//func TestConsensus_POS_Unscheduled(t *testing.T) {
//	thor.MockBlocklist([]string{})
//	chain := newHayabusaSetup(t, 2, 2)
//	signer := genesis.DevAccounts()[0]
//
//	require.NoError(t, chain.MintBlock())
//	require.NoError(t, chain.MintBlock())
//	require.NoError(t, chain.AddValidators())
//	require.NoError(t, chain.MintBlock())
//	assertConsensus(t, chain, false, true)
//
//	best := chain.Repo().BestBlockSummary()
//	parent, err := chain.Repo().NewBestChain().GetBlockSummary(best.Header.Number() - 1)
//	assert.NoError(t, err)
//
//	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, chain.GetForkConfig(), 0)
//	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval(), 10_000_000)
//	assert.NoError(t, err)
//	blk, _, _, err := flow.Pack(signer.PrivateKey, 0, false)
//	assert.NoError(t, err)
//
//	con := consensus.New(chain.Repo(), chain.Stater(), chain.GetForkConfig())
//	_, _, err = con.Process(parent, blk, 0, 0)
//
//	assert.ErrorContains(t, err, "block timestamp unscheduled")
//}
