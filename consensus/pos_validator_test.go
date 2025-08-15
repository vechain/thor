// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var minStake = big.NewInt(0).Mul(big.NewInt(25_000_000), big.NewInt(1e18))

func TestConsensus_PosFork(t *testing.T) {
	setup := newHayabusaSetup(t)

	// mint block 1: update the MBP
	setup.mintMbpBlock(1)

	// mint block 2: chain should set the staker contract, still using PoA
	best, parent, st := setup.mintBlock()
	leaders, err := builtin.Staker.Native(st).LeaderGroup()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(leaders))
	err = setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st), leaders)
	assert.Error(t, err)
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// mint block 3: add validator to the contract
	setup.mintAddValidatorBlock()

	// mint block 4: chain should switch to PoS
	best, parent, st = setup.mintBlock()
	leaders, err = builtin.Staker.Native(st).LeaderGroup()
	assert.NoError(t, err)
	err = setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st), leaders)
	assert.NoError(t, err)
}

func TestConsensus_POS_MissedSlots(t *testing.T) {
	setup := newHayabusaSetup(t)
	signer := genesis.DevAccounts()[0]

	setup.mintMbpBlock(1)              // mint block 1: update MBP
	setup.mintBlock()                  // mint block 2: set staker contract
	setup.mintAddValidatorBlock()      // mint block 3: add validator to queue
	setup.mintBlock()                  // mint block 4: chain should switch to PoS on future blocks
	_, parent, st := setup.mintBlock() // mint block 5: Full PoS

	blkPacker := packer.New(setup.chain.Repo(), setup.chain.Stater(), signer.Address, &signer.Address, setup.config, 0)
	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval*2, 10_000_000)
	assert.NoError(t, err)
	blk, stage, receipts, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)
	assert.NoError(t, setup.chain.AddBlock(blk, stage, receipts))

	leaders, err := builtin.Staker.Native(st).LeaderGroup()
	assert.NoError(t, err)
	err = setup.consensus.validateStakingProposer(blk.Header(), parent.Header, builtin.Staker.Native(st), leaders)
	assert.NoError(t, err)
	staker := builtin.Staker.Native(st)
	validator, err := staker.Get(signer.Address)
	assert.NoError(t, err)
	assert.Nil(t, validator.OfflineBlock)
}

func TestConsensus_POS_Unscheduled(t *testing.T) {
	setup := newHayabusaSetup(t)
	signer := genesis.DevAccounts()[0]

	setup.mintMbpBlock(1)              // mint block 1: update MBP
	setup.mintBlock()                  // mint block 2: set staker contract
	setup.mintAddValidatorBlock()      // mint block 3: add validator to queue
	setup.mintBlock()                  // mint block 4: chain should switch to PoS on future blocks
	_, parent, st := setup.mintBlock() // mint block 5: Full PoS

	blkPacker := packer.New(setup.chain.Repo(), setup.chain.Stater(), signer.Address, &signer.Address, setup.config, 0)
	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+1, 10_000_000)
	assert.NoError(t, err)
	blk, _, _, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)

	leaders, err := builtin.Staker.Native(st).LeaderGroup()
	assert.NoError(t, err)
	err = setup.consensus.validateStakingProposer(blk.Header(), parent.Header, builtin.Staker.Native(st), leaders)
	assert.ErrorContains(t, err, "block timestamp unscheduled")
}

type hayabusaSetup struct {
	chain     *testchain.Chain
	consensus *Consensus
	t         *testing.T
	config    *thor.ForkConfig
}

func newHayabusaSetup(t *testing.T) *hayabusaSetup {
	config := &thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config, 1)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)

	return &hayabusaSetup{
		chain:     chain,
		consensus: consensus,
		t:         t,
		config:    config,
	}
}

func (h *hayabusaSetup) mintBlock(txs ...*tx.Transaction) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	signer := genesis.DevAccounts()[0]
	assert.NoError(h.t, h.chain.MintBlock(signer, txs...))

	best := h.chain.Repo().BestBlockSummary()
	parent, err := h.chain.Repo().GetBlockSummary(best.Header.ParentID())
	assert.NoError(h.t, err)

	st := h.chain.Stater().NewState(parent.Root())
	_, err = builtin.Staker.Native(st).EvaluateOrUpdate(h.config, best.Header.Number())
	assert.NoError(h.t, err)

	// actualGroup, err := builtin.Staker.Native(st).LeaderGroup()
	// assert.NoError(h.t, err)
	// eq := reflect.DeepEqual(activeGroup, actualGroup)
	// assert.True(h.t, eq)
	// assert.Equal(h.t, activeGroup, actualGroup)

	return best, parent, st
}

func (h *hayabusaSetup) mintMbpBlock(amount int64) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	contract := h.chain.Contract(builtin.Params.Address, builtin.Params.ABI, genesis.DevAccounts()[0])
	tx, err := contract.BuildTransaction("set", big.NewInt(0), thor.KeyMaxBlockProposers, big.NewInt(amount))
	assert.NoError(h.t, err)
	return h.mintBlock(tx)
}

func (h *hayabusaSetup) mintAddValidatorBlock(accs ...genesis.DevAccount) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	if len(accs) == 0 {
		accs = make([]genesis.DevAccount, 1)
		accs[0] = genesis.DevAccounts()[0]
	}
	txs := make([]*tx.Transaction, 0, len(accs))
	contract := h.chain.Contract(builtin.Staker.Address, builtin.Staker.ABI, genesis.DevAccounts()[0])
	for _, acc := range accs {
		contract = contract.Attach(acc)
		tx, err := contract.BuildTransaction("addValidation", minStake, acc.Address, uint32(360)*24*7)
		assert.NoError(h.t, err)
		txs = append(txs, tx)
	}
	return h.mintBlock(txs...)
}
