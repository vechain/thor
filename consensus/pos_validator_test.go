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

var (
	minStake = big.NewInt(0).Mul(big.NewInt(25_000_000), big.NewInt(1e18))
)

func TestConsensus_PosFork(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)

	// mint block 1: update the MBP
	mintMbpBlock(t, chain, 1)

	// mint block 2: chain should set the staker contract, still using PoA
	best, parent, st := mintBlock(t, chain)
	_, err = consensus.validateStakingProposer(best.Header, parent.Header, st)
	assert.Error(t, err)
	_, err = consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// mint block 3: add validator to the contract
	mintAddValidatorBlock(t, chain)

	// mint block 4: chain should switch from PoA
	mintBlock(t, chain)

	// mint block 5: chain should switch to PoS
	best, parent, st = mintBlock(t, chain)
	_, err = consensus.validateStakingProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)
}

func TestConsensus_POS_MissedSlots(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)
	signer := genesis.DevAccounts()[0]

	mintMbpBlock(t, chain, 1)            // mint block 1: update MBP
	mintBlock(t, chain)                  // mint block 2: set staker contract
	mintAddValidatorBlock(t, chain)      // mint block 3: add validator to queue
	mintBlock(t, chain)                  // mint block 4: chain should switch to PoS on future blocks
	_, parent, st := mintBlock(t, chain) // mint block 5: Full PoS

	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, config)
	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval*2, 10_000_000)
	assert.NoError(t, err)
	blk, stage, receipts, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)
	assert.NoError(t, chain.AddBlock(blk, stage, receipts))

	_, err = consensus.validateStakingProposer(blk.Header(), parent.Header, st)
	assert.NoError(t, err)
	staker := builtin.Staker.Native(st)
	validator, _, err := staker.LookupMaster(signer.Address)
	assert.NoError(t, err)
	assert.True(t, validator.Online)
}

func TestConsensus_POS_Unscheduled(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)
	signer := genesis.DevAccounts()[0]

	mintMbpBlock(t, chain, 1)            // mint block 1: update MBP
	mintBlock(t, chain)                  // mint block 2: set staker contract
	mintAddValidatorBlock(t, chain)      // mint block 3: add validator to queue
	mintBlock(t, chain)                  // mint block 4: chain should switch to PoS on future blocks
	_, parent, st := mintBlock(t, chain) // mint block 5: Full PoS

	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, config)
	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+1, 10_000_000)
	assert.NoError(t, err)
	blk, _, _, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)

	_, err = consensus.validateStakingProposer(blk.Header(), parent.Header, st)
	assert.ErrorContains(t, err, "block timestamp unscheduled")
}

func mintBlock(t *testing.T, chain *testchain.Chain, txs ...*tx.Transaction) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	signer := genesis.DevAccounts()[0]
	assert.NoError(t, chain.MintBlock(signer, txs...))

	best := chain.Repo().BestBlockSummary()
	parent, err := chain.Repo().GetBlockSummary(best.Header.ParentID())
	assert.NoError(t, err)

	return best, parent, chain.Stater().NewState(parent.Root())
}

func mintMbpBlock(t *testing.T, chain *testchain.Chain, amount int64) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	contract := chain.Contract(builtin.Params.Address, builtin.Params.ABI, genesis.DevAccounts()[0])
	tx, err := contract.BuildTransaction("set", big.NewInt(0), thor.KeyMaxBlockProposers, big.NewInt(amount))
	assert.NoError(t, err)
	return mintBlock(t, chain, tx)
}

func mintAddValidatorBlock(t *testing.T, chain *testchain.Chain, accs ...genesis.DevAccount) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	if len(accs) == 0 {
		accs = make([]genesis.DevAccount, 1)
		accs[0] = genesis.DevAccounts()[0]
	}
	txs := make([]*tx.Transaction, 0, len(accs))
	contract := chain.Contract(builtin.Staker.Address, builtin.Staker.ABI, genesis.DevAccounts()[0])
	for _, acc := range accs {
		contract = contract.Attach(acc)
		tx, err := contract.BuildTransaction("addValidator", minStake, acc.Address, uint32(360)*24*7, true)
		assert.NoError(t, err)
		txs = append(txs, tx)
	}
	return mintBlock(t, chain, txs...)
}
