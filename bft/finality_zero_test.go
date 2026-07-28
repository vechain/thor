// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bft

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// buildGenesisPoSBFT builds a customnet with BFT+PoS active from genesis:
// FINALITY=0, HAYABUSA=0, HayabusaTP()=0, and a genesis staker set so that
// block proposers resolve to registered validators at the genesis committee.
func buildGenesisPoSBFT(t *testing.T) *TestBFT {
	// PoS active from genesis requires HayabusaTP()==0 (see genesis/customnet.go isPoSGenesis).
	orig := thor.HayabusaTP()
	tp := uint32(0)
	thor.SetConfig(thor.Config{HayabusaTP: &tp})
	t.Cleanup(func() { o := orig; thor.SetConfig(thor.Config{HayabusaTP: &o}) })

	// Value copy of NoFork (not &thor.NoFork) so we do not mutate the shared
	// package global that other tests in this package read.
	fcVal := thor.NoFork
	fcVal.FINALITY = 0
	fcVal.HAYABUSA = 0
	fc := &fcVal

	db := muxdb.NewMem()

	auth := make([]genesis.Authority, 0, len(devAccounts))
	accounts := make([]genesis.Account, 0, len(devAccounts))
	stakers := make([]genesis.Validator, 0, len(devAccounts))
	bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
	for _, acc := range devAccounts {
		auth = append(auth, genesis.Authority{
			MasterAddress:   acc.Address,
			EndorsorAddress: acc.Address,
			Identity:        thor.BytesToBytes32([]byte("master")),
		})
		accounts = append(accounts, genesis.Account{
			Address: acc.Address,
			Balance: (*genesis.HexOrDecimal256)(bal),
			Energy:  (*genesis.HexOrDecimal256)(bal),
		})
		stakers = append(stakers, genesis.Validator{Master: acc.Address, Endorser: acc.Address})
	}
	mbp := uint64(MaxBlockProposers)
	genConfig := genesis.CustomGenesis{
		LaunchTime: 1526400000,
		GasLimit:   thor.InitialGasLimit,
		ForkConfig: fc,
		Authority:  auth,
		Accounts:   accounts,
		Stakers:    stakers,
		Params:     genesis.Params{MaxBlockProposers: &mbp},
	}

	builder, err := genesis.NewCustomNet(&genConfig)
	require.NoError(t, err)

	stater := state.NewStater(db)
	gene, _, _, err := builder.Build(stater)
	require.NoError(t, err)

	repo, err := chain.NewRepository(db, gene)
	require.NoError(t, err)

	engine, err := NewEngine(repo, db, fc, devAccounts[len(devAccounts)-1].Address)
	require.NoError(t, err)
	_, err = engine.ShouldVote(repo.NewBestChain().GenesisID())
	require.NoError(t, err)

	return &TestBFT{engine: engine, db: db, repo: repo, stater: stater, fc: fc}
}

// TestComputeState_FinalityZero_DoesNotWalkGenesis reproduces the customnet crash
// "bft select: signer 0x00…00 absent from committee at block 0" and asserts it is fixed.
func TestComputeState_FinalityZero_DoesNotWalkGenesis(t *testing.T) {
	testBFT := buildGenesisPoSBFT(t)

	// Produce a few blocks; they stay in the first epoch (checkpoint == genesis),
	// so computeState's walk would descend toward block 0.
	parent := testBFT.repo.BestBlockSummary()
	for range 3 {
		next, err := testBFT.transitionToPosBlock(parent, devAccounts[0], false, true)
		require.NoError(t, err)
		parent = next
	}

	// Before the fix the walk descended to genesis and this errored with
	// "absent from committee at block 0"; after the fix it stops at block 1.
	_, err := testBFT.engine.computeState(testBFT.repo.BestBlockSummary())
	require.NoError(t, err)
}
