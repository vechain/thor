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
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	config.BLOCKLIST = math.MaxUint32

	chain, err := testchain.NewWithFork(config, 1)
	assert.NoError(t, err)

	// mint block 1: using PoA
	root := chain.Repo().BestBlockSummary().Root()
	packMbpBlock(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 2: deploy staker contract, still using PoA
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 3: add validator tx
	root = chain.Repo().BestBlockSummary().Root()
	packAddValidatorBlock(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 4: should switch to PoS
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 5: full PoS
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, false, root)
}

func packNext(t *testing.T, chain *testchain.Chain, interval uint64, txs ...*tx.Transaction) {
	account := genesis.DevAccounts()[0]
	p := packer.New(chain.Repo(), chain.Stater(), account.Address, &account.Address, chain.GetForkConfig(), 0)
	parent := chain.Repo().BestBlockSummary()
	flow, _, err := p.Schedule(parent, parent.Header.Timestamp()+interval)
	assert.NoError(t, err)

	for _, trx := range txs {
		assert.NoError(t, flow.Adopt(trx))
	}

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
	tx, err := contract.BuildTransaction("addValidation", vet, genesis.DevAccounts()[0].Address, uint32(360)*24*7)
	if err != nil {
		t.Fatal(err)
	}

	packNext(t, chain, interval, tx)
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
			cfg := thor.SoloFork
			cfg.HAYABUSA = tc.hayabusa
			hayabusaTP := uint32(1)
			thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

			chain, err := testchain.NewWithFork(&cfg, 1)
			assert.NoError(t, err)

			packNext(t, chain, thor.BlockInterval())
			packNext(t, chain, thor.BlockInterval())

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
	config := &thor.SoloFork
	config.HAYABUSA = 2
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	config.BLOCKLIST = math.MaxUint32

	chain, err := testchain.NewWithFork(config, 1)
	assert.NoError(t, err)

	// mint block 1: using PoA
	root := chain.Repo().BestBlockSummary().Root()
	packMbpBlock(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 2: deploy staker contract, still using PoA
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 3: add validator tx
	root = chain.Repo().BestBlockSummary().Root()
	packAddValidatorBlock(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	// mint block 4: should switch to PoS
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t, chain, thor.BlockInterval())
	verifyMechanism(t, chain, true, root)

	oldStakerBalance, err := chain.Stater().NewState(root).GetBalance(builtin.Staker.Address)
	assert.NoError(t, err)
	oldBalance, err := chain.Stater().NewState(root).GetBalance(genesis.DevAccounts()[1].Address)
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
	root = chain.Repo().BestBlockSummary().Root()
	packNext(t,
		chain,
		thor.BlockInterval(),
		failingTransaction,
	)
	verifyMechanism(t, chain, false, root)

	stakerBalance, err := chain.Stater().NewState(root).GetBalance(builtin.Staker.Address)
	assert.NoError(t, err)
	assert.Equal(t, oldStakerBalance, stakerBalance)

	balance, err := chain.Stater().NewState(root).GetBalance(genesis.DevAccounts()[1].Address)
	assert.NoError(t, err)
	assert.Equal(t, oldBalance, balance)
}
