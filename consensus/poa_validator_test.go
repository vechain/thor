// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func toWei(vet uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(vet), big.NewInt(1e18))
}

func toVet(wei *big.Int) uint64 {
	return wei.Div(wei, big.NewInt(1e18)).Uint64()
}

func TestAuthority_Hayabusa_TransitionPeriod(t *testing.T) {
	setup := newHayabusaSetup(t)

	// mint block 1: PoA - update the MBP
	blk, _, _ := setup.mintMbpBlock(1)

	endorsorBal, err := getEndorsorBalance(blk.Header, setup.chain)
	assert.NoError(t, err)

	// mint block 2: chain should set the staker contract, still using PoA
	best, parent, st := setup.mintBlock()
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// mint block 3: validator moves their stake to the contract
	best, parent, st = setup.mintAddValidatorBlock()
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// check the endorsor balance has reduced
	newEndorsorBal, err := getEndorsorBalance(best.Header, setup.chain)
	assert.NoError(t, err)
	assert.Equal(t, toVet(newEndorsorBal)+minStake, toVet(endorsorBal))

	// check the staker contract has the correct stake
	masterStake, err := getMasterStake(setup.chain, blk.Header)
	assert.NoError(t, err)
	assert.Equal(t, masterStake.QueuedVET, minStake)
}

func TestAuthority_Hayabusa_NegativeCases(t *testing.T) {
	setup := newHayabusaSetup(t)

	_, _, _ = setup.mintMbpBlock(1)

	best, parent, st := setup.mintBlock()
	st.SetRawStorage(builtin.Params.Address, thor.KeyProposerEndorsement, rlp.RawValue{0x0})

	_, err := setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.Error(t, err)

	headKey := thor.Blake2b([]byte("head"))
	st.SetRawStorage(builtin.Authority.Address, headKey, rlp.RawValue{0xFF})
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.Error(t, err)
}

func TestAuthorityCacheHandler_Success(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}
	mockForkConfig.HAYABUSA = 0

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
		{
			NodeMaster: thor.BytesToAddress([]byte("master2")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor2")),
			Identity:   thor.BytesToBytes32([]byte("identity2")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Staker.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("event1")),
							},
						},
					},
				},
			},
		},
	}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_NilCandidates(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{}

	err := consensus.authorityCacheHandler(nil, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_EmptyCandidates(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidates := poa.NewCandidates([]*authority.Candidate{})

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithAuthorityEvents(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Authority.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("candidate_added")),
								thor.BytesToBytes32(thor.BytesToAddress([]byte("new_candidate")).Bytes()),
							},
						},
					},
				},
			},
		},
	}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithStakerEvents(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Staker.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("staker_event")),
							},
						},
					},
				},
			},
		},
	}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithParamsEvents(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: builtin.Params.Address,
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("params_event")),
							},
						},
					},
				},
			},
		},
	}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithEndorsorTransfers(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Transfers: []*tx.Transfer{
						{
							Sender:    thor.BytesToAddress([]byte("endorsor1")),
							Recipient: thor.BytesToAddress([]byte("recipient")),
							Amount:    big.NewInt(1000),
						},
					},
				},
			},
		},
	}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithMultipleEvents(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{
		&tx.Receipt{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: thor.BytesToAddress([]byte("contract1")),
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("event1")),
							},
						},
						{
							Address: thor.BytesToAddress([]byte("contract2")),
							Topics: []thor.Bytes32{
								thor.BytesToBytes32([]byte("event2")),
							},
						},
					},
				},
			},
		},
	}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithNilReceipts(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	err := consensus.authorityCacheHandler(candidates, header, nil)
	assert.NoError(t, err)
}

func TestAuthorityCacheHandler_WithEmptyReceipts(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	candidateList := []*authority.Candidate{
		{
			NodeMaster: thor.BytesToAddress([]byte("master1")),
			Endorsor:   thor.BytesToAddress([]byte("endorsor1")),
			Identity:   thor.BytesToBytes32([]byte("identity1")),
			Active:     true,
		},
	}

	candidates := poa.NewCandidates(candidateList)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	receipts := tx.Receipts{}

	err := consensus.authorityCacheHandler(candidates, header, receipts)
	assert.NoError(t, err)
}

func TestAuthorityBalanceCheck_BeforeHayabusaFork_AccountBalanceSufficient(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(2000))

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.True(t, hasBalance, "Should have sufficient account balance before HAYABUSA fork")
}

func TestAuthorityBalanceCheck_BeforeHayabusaFork_AccountBalanceInsufficient(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent123"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(500))

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.False(t, hasBalance, "Should not have sufficient account balance before HAYABUSA fork")
}

func TestAuthorityBalanceCheck_AfterHayabusaFork_AccountBalanceSufficient(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("block150"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(2000))

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.True(t, hasBalance, "Should have sufficient account balance after HAYABUSA fork")
}

func TestAuthorityBalanceCheck_AfterHayabusaFork_AccountBalanceInsufficient_StakeInsufficient(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("block150"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(500))

	stakerAddr := builtin.Staker.Address
	st.SetCode(stakerAddr, builtin.Staker.RuntimeBytecodes())

	validator := &validation.Validation{
		Endorser:           endorsor,
		Beneficiary:        nil,
		Period:             0,
		CompleteIterations: 0,
		Status:             0,
		StartBlock:         0,
		ExitBlock:          nil,
		OfflineBlock:       nil,
		LockedVET:          0,
		PendingUnlockVET:   0,
		QueuedVET:          500,
		CooldownVET:        0,
		WithdrawableVET:    0,
		Weight:             100,
	}

	slot := thor.Blake2b(signer.Bytes(), thor.BytesToBytes32([]byte("validations")).Bytes())
	validatorData, err := rlp.EncodeToBytes(validator)
	assert.NoError(t, err)
	st.SetRawStorage(stakerAddr, slot, validatorData)

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.False(t, hasBalance, "Should not have sufficient balance or stake after HAYABUSA fork")
}

func TestAuthorityBalanceCheck_AfterHayabusaFork_AccountBalanceInsufficient_NoValidatorEntry(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("block150"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(500))

	stakerAddr := builtin.Staker.Address
	st.SetCode(stakerAddr, builtin.Staker.RuntimeBytecodes())

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.False(t, hasBalance, "Should not have sufficient balance when no validator entry exists")
}

func TestAuthorityBalanceCheck_AfterHayabusaFork_AccountBalanceInsufficient_EmptyValidatorEntry(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("block150"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(500))

	stakerAddr := builtin.Staker.Address
	st.SetCode(stakerAddr, builtin.Staker.RuntimeBytecodes())

	validator := &validation.Validation{
		Endorser:           thor.Address{},
		Beneficiary:        nil,
		Period:             0,
		CompleteIterations: 0,
		Status:             0,
		StartBlock:         0,
		ExitBlock:          nil,
		OfflineBlock:       nil,
		LockedVET:          0,
		PendingUnlockVET:   0,
		QueuedVET:          0,
		CooldownVET:        0,
		WithdrawableVET:    0,
		Weight:             0,
	}

	slot := thor.Blake2b(signer.Bytes(), thor.BytesToBytes32([]byte("validations")).Bytes())
	validatorData, err := rlp.EncodeToBytes(validator)
	assert.NoError(t, err)
	st.SetRawStorage(stakerAddr, slot, validatorData)

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.False(t, hasBalance, "Should not have sufficient balance when validator entry is empty")
}

func TestAuthorityBalanceCheck_AfterHayabusaFork_AccountBalanceInsufficient_NilQueuedVET(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}

	consensus := New(mockRepo, stater, mockForkConfig)

	header := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("block150"))).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{}).
		Build().Header()

	st := stater.NewState(trie.Root{})
	signer := thor.BytesToAddress([]byte("signer"))
	endorsor := thor.BytesToAddress([]byte("endorsor"))
	minBalance := big.NewInt(1000)

	st.SetBalance(endorsor, big.NewInt(500))

	stakerAddr := builtin.Staker.Address
	st.SetCode(stakerAddr, builtin.Staker.RuntimeBytecodes())

	validator := &validation.Validation{
		Endorser:           endorsor,
		Beneficiary:        nil,
		Period:             0,
		CompleteIterations: 0,
		Status:             0,
		StartBlock:         0,
		ExitBlock:          nil,
		OfflineBlock:       nil,
		LockedVET:          0,
		PendingUnlockVET:   0,
		QueuedVET:          0,
		CooldownVET:        0,
		WithdrawableVET:    0,
		Weight:             100,
	}

	slot := thor.Blake2b(signer.Bytes(), thor.BytesToBytes32([]byte("validations")).Bytes())
	validatorData, err := rlp.EncodeToBytes(validator)
	assert.NoError(t, err)
	st.SetRawStorage(stakerAddr, slot, validatorData)

	checker := consensus.authorityBalanceCheck(header, st, signer)

	hasBalance, err := checker(endorsor, minBalance)

	assert.NoError(t, err)
	assert.False(t, hasBalance, "Should not have sufficient balance when QueuedVET is nil")
}

func getEndorsorBalance(blk *block.Header, chain *testchain.Chain) (*big.Int, error) {
	st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	signer, err := blk.Signer()
	if err != nil {
		return nil, err
	}
	_, endorsor, _, _, _ := builtin.Authority.Native(st).Get(signer)
	balance, err := st.GetBalance(endorsor)
	if err != nil {
		return nil, err
	}
	return balance, nil
}

func getMasterStake(chain *testchain.Chain, blk *block.Header) (*validation.Validation, error) {
	st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	signer, err := blk.Signer()
	if err != nil {
		return nil, err
	}
	staker := builtin.Staker.Native(st)
	validator, err := staker.GetValidation(signer)
	return validator, err
}
