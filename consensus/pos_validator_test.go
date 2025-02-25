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
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestConsensus_PosFork(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)
	mintBlock(t, chain) // mint block 1 with new authorities

	// mint block 2: chain should fork but still use PoA for consensus
	best, parent, st := mintBlock(t, chain)
	_, err = consensus.validateStakingProposer(best.Header, parent.Header, st)
	assert.Error(t, err)
	_, err = consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// mint block 3: chain should be using PoS for consensus
	best, parent, st = mintBlock(t, chain)
	_, err = consensus.validateStakingProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)
	_, err = consensus.validateProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)
	_, err = consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.Error(t, err)
}

func TestConsensus_POS_MissedSlots(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)
	signer := genesis.DevAccounts()[0]

	mintBlock(t, chain)                  // mint block 1
	mintBlock(t, chain)                  // mint block 2
	_, parent, st := mintBlock(t, chain) // mint block 3

	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, config)
	flow, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval*2, 10_000_000)
	assert.NoError(t, err)
	blk, stage, receipts, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)
	assert.NoError(t, chain.AddBlock(blk, stage, receipts))

	_, err = consensus.validateStakingProposer(blk.Header(), parent.Header, st)
	assert.NoError(t, err)
	staker := builtin.Staker.Native(st)
	validator, err := staker.Get(signer.Address)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), validator.MissedSlots)
}

func TestConsensus_POS_Unscheduled(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)
	signer := genesis.DevAccounts()[0]

	mintBlock(t, chain)                  // mint block 1
	mintBlock(t, chain)                  // mint block 2
	_, parent, st := mintBlock(t, chain) // mint block 3

	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, config)
	flow, err := blkPacker.Mock(parent, parent.Header.Timestamp()+1, 10_000_000)
	assert.NoError(t, err)
	blk, _, _, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)

	_, err = consensus.validateStakingProposer(blk.Header(), parent.Header, st)
	assert.ErrorContains(t, err, "block timestamp unscheduled")
}

func TestConsensus_POS_BadScore(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)
	signer := genesis.DevAccounts()[1]

	trx := addAuthorityTx(t, chain.ChainTag(), genesis.DevAccounts()[1].Address)

	mintBlock(t, chain, trx)             // mint block 1 - PoA
	mintBlock(t, chain)                  // mint block 2 - PoA - Fork Happens
	_, parent, st := mintBlock(t, chain) // mint block 3 - PoS

	blkPacker := packer.New(chain.Repo(), chain.Stater(), signer.Address, &signer.Address, config)
	flow, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval, 10_000_000)
	assert.NoError(t, err)
	blk, _, _, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)

	// Add a new staker to the state, so that the block score is invalid
	staker := builtin.Staker.Native(st)
	signer2 := genesis.DevAccounts()[2]
	assert.NoError(t, staker.AddValidator(
		parent.Header.Number(),
		signer2.Address,
		signer2.Address,
		parent.Header.Number()+uint32(360)*24*14,
		big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))))
	assert.NoError(t, staker.ActivateNextValidator())

	_, err = consensus.validateStakingProposer(blk.Header(), parent.Header, st)
	assert.ErrorContains(t, err, "block total score invalid")
}

func mintBlock(t *testing.T, chain *testchain.Chain, txs ...*tx.Transaction) (*chain.BlockSummary, *chain.BlockSummary, *state.State) {
	signer := genesis.DevAccounts()[0]
	assert.NoError(t, chain.MintBlock(signer, txs...))

	best := chain.Repo().BestBlockSummary()
	parent, err := chain.Repo().GetBlockSummary(best.Header.ParentID())
	assert.NoError(t, err)

	return best, parent, chain.Stater().NewState(parent.Root())
}

func addAuthorityTx(t *testing.T, chainTag byte, addr thor.Address) *tx.Transaction {
	authAdd, ok := builtin.Authority.ABI.MethodByName("add")
	assert.True(t, ok)
	addData, err := authAdd.EncodeInput(addr, addr, datagen.RandomHash())
	assert.NoError(t, err)
	clause := tx.NewClause(&builtin.Authority.Address).WithData(addData)
	trx := new(tx.Builder).
		Clause(clause).
		Gas(1_000_000).
		ChainTag(chainTag).
		Expiration(100000).
		Build()
	trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)

	return trx
}
