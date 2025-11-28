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
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestFlow_Schedule_POS(t *testing.T) {
	forkConfig := &thor.SoloFork
	forkConfig.HAYABUSA = 2
	forkConfig.BLOCKLIST = math.MaxUint32
	cfg := genesis.SoloConfig
	cfg.EpochLength = 1

	devConfig := genesis.DevConfig{
		ForkConfig: forkConfig,
		Config:     &cfg,
	}

	chain, err := testchain.NewIntegrationTestChain(devConfig, 2)
	assert.NoError(t, err)

	// mint block 1: using PoA
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, true, false)

	// mint block 2: deploy staker contract, still using PoA
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, true, true)

	// mint block 3: add validator tx
	require.NoError(t, chain.AddValidators())
	verifyMechanism(t, chain, true, true)

	// mint block 4: should switch to PoS
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, false, true)

	// mint block 5: full PoS
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, false, true)
}

func verifyMechanism(t *testing.T, chain *testchain.Chain, expectPoA bool, stakerDeployed bool) {
	best := chain.Repo().BestBlockSummary()
	st := chain.Stater().NewState(best.Root())
	staker := builtin.Staker.Native(st)
	active, err := staker.IsPoSActive()
	require.NoError(t, err)
	if expectPoA {
		require.False(t, active, "expected PoA mechanism")
	} else {
		require.True(t, active, "expected PoS mechanism")
	}
	bytecode, err := st.GetCode(builtin.Staker.Address)
	require.NoError(t, err)
	if stakerDeployed {
		require.NotEmpty(t, bytecode, "staker contract should be deployed")
	} else {
		require.Empty(t, bytecode, "staker contract should not be deployed")
	}
}

func TestPacker_StopsEnergyAtHardfork(t *testing.T) {
	cases := []struct {
		name       string
		hayabusa   uint32
		expectStop bool
	}{
		{"stops at hardfork block", 2, true},
		{"does not stop without fork", math.MaxUint32, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &thor.SoloFork
			cfg.HAYABUSA = tc.hayabusa
			hayabusaTP := uint32(1)
			thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

			chain, err := testchain.NewWithFork(cfg, 10)
			assert.NoError(t, err)

			require.NoError(t, chain.MintBlock())
			require.NoError(t, chain.MintBlock())

			best := chain.Repo().BestBlockSummary()
			st := chain.Stater().NewState(best.Root())
			stop, err := builtin.Energy.Native(st, best.Header.Timestamp()).GetEnergyGrowthStopTime()
			assert.NoError(t, err)
			if tc.expectStop {
				assert.Equal(t, best.Header.Timestamp(), stop)
			} else {
				assert.Equal(t, uint64(math.MaxUint64), stop)
			}
		})
	}
}

func TestFlow_Revert(t *testing.T) {
	forkConfig := &thor.SoloFork
	forkConfig.HAYABUSA = 2
	forkConfig.BLOCKLIST = math.MaxUint32
	cfg := genesis.SoloConfig
	cfg.EpochLength = 1

	devConfig := genesis.DevConfig{
		ForkConfig: forkConfig,
		Config:     &cfg,
	}

	chain, err := testchain.NewIntegrationTestChain(devConfig, 2)
	assert.NoError(t, err)

	// mint block 1: using PoA
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, true, false)

	// mint block 2: deploy staker contract, still using PoA
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, true, true)

	// mint block 3: add validator tx
	require.NoError(t, chain.AddValidators())
	verifyMechanism(t, chain, true, true)

	// mint block 4: should switch to PoS
	require.NoError(t, chain.MintBlock())
	verifyMechanism(t, chain, false, true)

	oldStakerBalance, err := chain.State().GetBalance(builtin.Staker.Address)
	assert.NoError(t, err)
	oldBalance, err := chain.State().GetBalance(genesis.DevAccounts()[1].Address)
	assert.NoError(t, err)

	bestBlock, _ := chain.BestBlock()
	amount, _ := big.NewInt(0).SetString("1000000000000000000", 10)
	failingTransaction := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(chain.Repo().ChainTag()).
		Expiration(10).
		Nonce(1).
		Gas(3000000).
		MaxFeePerGas(bestBlock.Header().BaseFee()).
		MaxPriorityFeePerGas(big.NewInt(3000000)).
		Clause(tx.NewClause(&builtin.Staker.Address).WithData([]byte{
			0xc3, 0xc4, 0xb1, 0x38, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0xf0, 0x77, 0xb4, 0x91,
			0xb3, 0x55, 0xe6, 0x40, 0x48, 0xce, 0x21, 0xe3,
			0xa6, 0xfc, 0x47, 0x51, 0xee, 0xea, 0x77, 0xfa,

			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0e, 0x10,
		}).WithValue(amount)).
		Clause(tx.NewClause(&genesis.DevAccounts()[1].Address).WithValue(amount)).
		Build()
	failingTransaction = tx.MustSign(failingTransaction, genesis.DevAccounts()[0].PrivateKey)

	// mint block 5: full PoS
	require.NoError(t, chain.MintBlock(failingTransaction))
	verifyMechanism(t, chain, false, true)

	stakerBalance, err := chain.State().GetBalance(builtin.Staker.Address)
	assert.NoError(t, err)
	assert.Equal(t, oldStakerBalance, stakerBalance)

	balance, err := chain.State().GetBalance(genesis.DevAccounts()[1].Address)
	assert.NoError(t, err)
	assert.Equal(t, oldBalance, balance)
}
