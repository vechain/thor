// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

var minStake = uint64(25_000_000)

func TestConsensus_PosFork(t *testing.T) {
	setup := newHayabusaSetup(t)

	// mint block 1: update the MBP
	setup.mintMbpBlock(1)

	// mint block 2: chain should set the staker contract, still using PoA
	best, parent, st := setup.mintBlock()
	_, err := setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "pos - block signer invalid")
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// mint block 3: add validator to the contract
	setup.mintAddValidatorBlock()

	// mint block 4: chain should switch to PoS
	best, parent, st = setup.mintBlock()
	_, err = setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st))
	assert.NoError(t, err)

	cache, err := simplelru.NewLRU(16, nil)
	assert.NoError(t, err)

	staker := builtin.Staker.Native(st)
	leaders, err := staker.LeaderGroup()
	assert.NoError(t, err)

	signer, err := best.Header.Signer()
	assert.NoError(t, err)

	parentSig, err := parent.Header.Signer()
	assert.NoError(t, err)

	beneficiary := thor.BytesToAddress([]byte("test"))

	var newLeaders []validation.Leader
	for _, leader := range leaders {
		// delete(leaders, signer)
		if leader.Address == signer {
			continue
		}
		newLeaders = append(newLeaders, leader)
	}

	newLeaders = append(newLeaders, validation.Leader{
		Address:     parentSig,
		Beneficiary: &beneficiary,
		Endorser:    thor.Address{},
		Weight:      10,
		Active:      false,
	})
	cache.Add(parent.Header.ID(), newLeaders)
	setup.consensus.validatorsCache = cache

	newParentHeader := new(block.Builder).
		ParentID(parent.Header.ParentID()).
		Timestamp(parent.Header.Timestamp()).
		GasLimit(parent.Header.GasLimit()).
		GasUsed(parent.Header.GasUsed()).
		TotalScore(10003).
		StateRoot(parent.Header.StateRoot()).
		ReceiptsRoot(parent.Header.ReceiptsRoot()).
		Beneficiary(parent.Header.Beneficiary()).
		Build().Header()

	_, err = setup.consensus.validateStakingProposer(best.Header, newParentHeader, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "pos - stake beneficiary mismatch")

	newLeaders = make([]validation.Leader, 0, len(leaders))
	for _, leader := range newLeaders {
		if leader.Address == parentSig {
			newLeaders = append(newLeaders, validation.Leader{
				Address:     parentSig,
				Beneficiary: nil,
				Endorser:    thor.Address{},
				Weight:      10,
				Active:      false,
			})
		} else {
			newLeaders = append(newLeaders, leader)
		}
	}
	cache.Add(parent.Header.ID(), newLeaders)
	setup.consensus.validatorsCache = cache

	newParentHeader = new(block.Builder).
		ParentID(parent.Header.ParentID()).
		Timestamp(parent.Header.Timestamp()).
		GasLimit(parent.Header.GasLimit()).
		GasUsed(parent.Header.GasUsed()).
		TotalScore(1).
		StateRoot(parent.Header.StateRoot()).
		ReceiptsRoot(parent.Header.ReceiptsRoot()).
		Beneficiary(parent.Header.Beneficiary()).
		Build().Header()

	_, err = setup.consensus.validateStakingProposer(best.Header, newParentHeader, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "pos - block total score invalid")

	slotLockedVET := thor.BytesToBytes32([]byte(("total-weighted-stake")))
	st.SetRawStorage(builtin.Staker.Address, slotLockedVET, rlp.RawValue{0xFF})

	_, err = setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "pos - cannot get total weight")

	newParentHeader = new(block.Builder).
		ParentID(parent.Header.ParentID()).
		Timestamp(parent.Header.Timestamp()).
		GasLimit(parent.Header.GasLimit()).
		GasUsed(parent.Header.GasUsed()).
		TotalScore(10003).
		StateRoot(parent.Header.StateRoot()).
		ReceiptsRoot(parent.Header.ReceiptsRoot()).
		Beneficiary(parent.Header.Beneficiary()).
		Build().Header()

	slotValidation := thor.BytesToBytes32([]byte(("validations")))
	slot := thor.Blake2b(parentSig.Bytes(), slotValidation.Bytes())
	st.SetRawStorage(builtin.Staker.Address, slot, rlp.RawValue{0xFF})

	_, err = setup.consensus.validateStakingProposer(best.Header, newParentHeader, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "failed to get validator")
}

func TestConsensus_CannotGetLeaderGroup(t *testing.T) {
	setup := newHayabusaSetup(t)

	setup.mintMbpBlock(1)

	best, parent, st := setup.mintBlock()
	_, err := setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "pos - block signer invalid")
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	setup.mintAddValidatorBlock()

	best, parent, st = setup.mintBlock()
	_, err = setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st))
	assert.NoError(t, err)

	slotValidationsActiveHead := thor.BytesToBytes32([]byte("validations-active-head"))

	st.SetRawStorage(builtin.Staker.Address, slotValidationsActiveHead, rlp.RawValue{0xFF})

	_, err = setup.consensus.validateStakingProposer(best.Header, parent.Header, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "pos - cannot get leader group")
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
	flow, _, err := blkPacker.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval()*2, 10_000_000)
	assert.NoError(t, err)
	blk, stage, receipts, err := flow.Pack(signer.PrivateKey, 0, false)
	assert.NoError(t, err)
	assert.NoError(t, setup.chain.AddBlock(blk, stage, receipts))

	_, err = setup.consensus.validateStakingProposer(blk.Header(), parent.Header, builtin.Staker.Native(st))
	assert.NoError(t, err)
	staker := builtin.Staker.Native(st)
	validator, err := staker.GetValidation(signer.Address)
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

	_, err = setup.consensus.validateStakingProposer(blk.Header(), parent.Header, builtin.Staker.Native(st))
	assert.ErrorContains(t, err, "block timestamp unscheduled")
}

func TestValidateStakingProposer_LockedVETError(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	parent := &block.Header{}

	st := stater.NewState(trie.Root{})

	stakerAddr := builtin.Staker.Address
	st.SetCode(stakerAddr, builtin.Staker.RuntimeBytecodes())

	paramsAddr := builtin.Params.Address
	st.SetCode(paramsAddr, builtin.Params.RuntimeBytecodes())

	paramKey := thor.BytesToBytes32([]byte("some_param"))
	paramValue := []byte("valid_value")
	st.SetStorage(paramsAddr, paramKey, thor.BytesToBytes32(paramValue))

	staker := builtin.Staker.Native(st)

	builder := new(block.Builder).
		ParentID(thor.Bytes32{}).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{})

	blk := builder.Build()
	validSignature := make([]byte, 65)
	copy(validSignature, []byte("valid_signature_65_bytes_long_for_testing"))
	blk = blk.WithSignature(validSignature)
	header := blk.Header()

	_, err := consensus.validateStakingProposer(header, parent, staker)
	assert.ErrorContains(t, err, "pos - block signer invalid")
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
	_, err = builtin.Staker.Native(st).SyncPOS(h.config, best.Header.Number())
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
		tx, err := contract.BuildTransaction("addValidation", toWei(minStake), acc.Address, uint32(360)*24*7)
		assert.NoError(h.t, err)
		txs = append(txs, tx)
	}
	return h.mintBlock(txs...)
}
