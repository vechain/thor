// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"context"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func DebugRevert(t *testing.T, receipt *api.Receipt, sender *bind.MethodBuilder) {
	if receipt == nil {
		require.Fail(t, "receipt is nil")
		return
	}
	if receipt.Reverted {
		_, err := sender.Call().
			AtRevision(receipt.Meta.BlockID.String()).
			Caller(&receipt.Meta.TxOrigin).
			Execute()
		if err != nil {
			require.Fail(t, "transaction reverted", err)
		} else {
			require.Fail(t, "transaction reverted for unknown reason")
		}
	}
}

func ExpectRevert(t *testing.T, receipt *api.Receipt, sender *bind.MethodBuilder, expectedMessage string) {
	if receipt == nil {
		require.Fail(t, "receipt is nil")
		return
	}
	assert.True(t, receipt.Reverted, "expected transaction to revert but it did not")
	_, err := sender.Call().
		AtRevision(receipt.Meta.BlockID.String()).
		Caller(&receipt.Meta.TxOrigin).
		Execute()
	require.Contains(t, err.Error(), expectedMessage, "transaction did not revert as expected")
}

func TestStaker(t *testing.T) {
	minStakingPeriod := uint32(360) * 24 * 7 // 360 days in seconds

	node, client := newTestNode(t, false)
	defer node.Stop()

	// builtins
	staker, err := NewStaker(client)
	require.NoError(t, err)
	authority, err := NewAuthority(client)
	require.NoError(t, err)
	params, err := NewParams(client)
	require.NoError(t, err)

	// set authorities - required for initial staker setup
	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)
	stargate := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)
	validators := genesis.DevAccounts()[0:2]
	for _, acc := range genesis.DevAccounts()[1:] {
		method := authority.Add(acc.Address, acc.Address, datagen.RandomHash())
		receipt, _, err := method.
			Send().
			WithSigner(executor).
			WithOptions(txOpts()).
			SubmitAndConfirm(context.Background())
		assert.NoError(t, err)
		DebugRevert(t, receipt, method)
	}

	// set max block proposers
	if _, _, err := params.Set(thor.KeyMaxBlockProposers, big.NewInt(3)).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t)); err != nil {
		t.Fatal(err)
	}
	// set stargate address
	if _, _, err := params.Set(thor.KeyDelegatorContractAddress, new(big.Int).SetBytes(stargate.Address().Bytes())).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t)); err != nil {
		t.Fatal(err)
	}

	// add validators - trigger PoS activation
	minStake := MinStake()

	thor.SetConfig(thor.Config{
		EpochLength: 1,
	})
	for _, acc := range validators {
		method := staker.AddValidation(acc.Address, minStake, minStakingPeriod)
		receipt, _, err := method.
			Send().
			WithSigner(bind.NewSigner(acc.PrivateKey)).
			WithOptions(txOpts()).
			SubmitAndConfirm(txContext(t))
		require.NoError(t, err)
		DebugRevert(t, receipt, method)
	}

	// pack a new block
	require.NoError(t, node.Chain().MintBlock(genesis.DevAccounts()[0]))

	// TotalStake
	totalStake, totalWeight, err := staker.TotalStake()
	require.NoError(t, err)
	require.Equal(t, 1, totalWeight.Sign())
	require.Equal(t, new(big.Int).Mul(minStake, big.NewInt(2)), totalStake)

	// GetStake
	_, firstID, err := staker.FirstActive()
	require.NoError(t, err)
	getStakeRes, err := staker.GetValidation(firstID)
	require.NoError(t, err)
	require.False(t, getStakeRes.Address.IsZero())
	require.False(t, getStakeRes.Endorser.IsZero())
	require.Equal(t, getStakeRes.Stake, minStake)
	require.Equal(t, getStakeRes.Weight, minStake)
	require.Equal(t, getStakeRes.QueuedStake.String(), big.NewInt(0).String())
	require.True(t, getStakeRes.Exists())
	require.Equal(t, uint32(math.MaxUint32), getStakeRes.OfflineBlock)
	require.Equal(t, StakerStatusActive, getStakeRes.Status)
	require.True(t, getStakeRes.IsOnline())

	// GetPeriodDetails
	getPeriodDetailsRes, err := staker.GetValidationPeriodDetails(firstID)
	require.NoError(t, err)
	require.Equal(t, firstID, getPeriodDetailsRes.Address)
	require.Equal(t, minStakingPeriod, getPeriodDetailsRes.Period)
	require.Equal(t, uint32(15), getPeriodDetailsRes.StartBlock)
	require.Equal(t, uint32(math.MaxUint32), getPeriodDetailsRes.ExitBlock)
	require.Equal(t, uint32(0), getPeriodDetailsRes.CompletedPeriods)

	// FirstActive
	firstActive, firstID, err := staker.FirstActive()
	require.NoError(t, err)
	require.False(t, firstID.IsZero())
	require.True(t, firstActive.Exists())
	require.Equal(t, minStake, firstActive.Stake)
	require.Equal(t, minStake, firstActive.Weight)
	require.False(t, firstActive.Endorser.IsZero())

	// Next
	next, id, err := staker.Next(firstID)
	require.NoError(t, err)
	require.False(t, id.IsZero())
	nextStatus, err := staker.GetValidation(next.Address)
	require.True(t, next.Exists())
	require.NoError(t, err)
	require.Equal(t, StakerStatusActive, nextStatus.Status)
	require.Equal(t, minStake, next.Stake)
	require.Equal(t, minStake, next.Weight)
	require.False(t, next.Endorser.IsZero())

	var (
		validator    = genesis.DevAccounts()[9]
		validatorKey = bind.NewSigner(validator.PrivateKey)
	)

	// AddValidator
	receipt, _, err := staker.AddValidation(validator.Address, minStake, minStakingPeriod).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	maxUint32 := uint32(math.MaxUint32)
	nonExistentValidator, err := staker.GetValidation(thor.Address{})
	require.NoError(t, err)
	require.Equal(t, nonExistentValidator.Address, thor.Address{})
	require.Equal(t, nonExistentValidator.Stake.String(), big.NewInt(0).String())
	require.Equal(t, nonExistentValidator.Weight.String(), big.NewInt(0).String())
	require.Equal(t, nonExistentValidator.QueuedStake.String(), big.NewInt(0).String())
	require.Equal(t, nonExistentValidator.Status, StakerStatusUnknown)
	require.Equal(t, nonExistentValidator.OfflineBlock, maxUint32)

	nonExistentDelegator, err := staker.GetDelegation(big.NewInt(6))
	require.NoError(t, err)
	require.Equal(t, nonExistentDelegator.Validator, thor.Address{})
	require.Equal(t, nonExistentDelegator.Stake.String(), big.NewInt(0).String())
	require.Equal(t, nonExistentDelegator.Multiplier, uint8(0))
	require.False(t, nonExistentDelegator.Locked)

	queuedEvents, err := staker.FilterValidatorQueued(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, queuedEvents, 1)
	require.Equal(t, validator.Address, queuedEvents[0].Endorser)
	require.Equal(t, minStake, queuedEvents[0].Stake)
	queuedID := queuedEvents[0].Node

	// FirstQueued
	firstQueued, id, err := staker.FirstQueued()
	require.NoError(t, err)
	require.False(t, id.IsZero())
	firstQueuedStatus, err := staker.GetValidation(firstQueued.Address)
	require.NoError(t, err)
	require.True(t, firstQueued.Exists())
	require.Equal(t, 0, firstQueued.Stake.Sign())
	require.Equal(t, StakerStatusQueued, firstQueuedStatus.Status)
	require.False(t, firstQueued.Endorser.IsZero())

	// TotalQueued
	queuedStake, err := staker.QueuedStake()
	require.NoError(t, err)
	stake := big.NewInt(0).Mul(big.NewInt(1e18), big.NewInt(25))
	stake = big.NewInt(0).Mul(stake, big.NewInt(1e6))
	require.Equal(t, stake, queuedStake)

	thor.SetConfig(thor.Config{
		EpochLength: 180,
	})

	// IncreaseStake
	receipt, _, err = staker.IncreaseStake(queuedID, minStake).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	increaseEvents, err := staker.FilterStakeIncreased(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, increaseEvents, 1)
	require.Equal(t, validator.Address, increaseEvents[0].Validator)
	require.Equal(t, minStake, increaseEvents[0].Added)

	// DecreaseStake
	receipt, _, err = staker.DecreaseStake(queuedID, minStake).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	decreaseEvents, err := staker.FilterStakeDecreased(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, decreaseEvents, 1)
	require.Equal(t, queuedID, decreaseEvents[0].Validator)
	require.Equal(t, minStake, decreaseEvents[0].Removed)

	// SignalExit
	receipt, _, err = staker.SignalExit(queuedID).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	// No events for signal exit when state is queued
	autoRenewEvents, err := staker.FilterValidationSignaledExit(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, autoRenewEvents, 0)

	// AddDelegation
	receipt, _, err = staker.AddDelegation(queuedID, minStake, 100).
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	delegationEvents, err := staker.FilterDelegationAdded(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, delegationEvents, 1)
	delegationID := delegationEvents[0].DelegationID

	// GetDelegationStake
	delegationStake, err := staker.GetDelegation(delegationID)
	require.NoError(t, err)

	require.Equal(t, minStake, delegationStake.Stake)
	require.Equal(t, uint8(100), delegationStake.Multiplier)
	require.Equal(t, queuedID, delegationStake.Validator)
	require.False(t, delegationStake.Locked)

	// GetDelegationPeriodDetails
	delegationPeriodDetails, err := staker.GetDelegationPeriodDetails(delegationID)
	require.NoError(t, err)
	require.Equal(t, uint32(1), delegationPeriodDetails.StartPeriod)
	require.Equal(t, uint32(math.MaxUint32), delegationPeriodDetails.EndPeriod)

	// GetValidatorsTotals
	validationTotals, err := staker.GetValidationTotals(firstID)
	require.NoError(t, err)

	require.Equal(t, minStake, validationTotals.TotalLockedStake)
	require.Equal(t, minStake, validationTotals.TotalLockedWeight)
	require.Equal(t, big.NewInt(0).String(), validationTotals.TotalQueuedStake.String())
	require.Equal(t, big.NewInt(0).String(), validationTotals.TotalExitingStake.String())
	require.Equal(t, minStake, validationTotals.NextPeriodWeight)

	// GetValidationsNum
	active, queued, err := staker.GetValidationsNum()
	require.NoError(t, err)
	require.Equal(t, uint64(2), active)
	require.Equal(t, uint64(1), queued)

	// UpdateDelegationAutoRenew - Enable AutoRenew
	receipt, _, err = staker.SignalDelegationExit(delegationID).
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	ExpectRevert(t, receipt, staker.SignalDelegationExit(delegationID), "delegation has not started yet, funds can be withdrawn")

	// DelegationSignaledExit may not emit while queued; ensure no crash and zero events
	delegationSignaledExit, err := staker.FilterDelegationSignaledExit(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, delegationSignaledExit, 0)

	// Withdraw
	receipt, _, err = staker.WithdrawStake(queuedID).Send().WithSigner(validatorKey).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	withdrawEvents, err := staker.FilterValidationWithdrawn(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, withdrawEvents, 1)

	getStatusRes, err := staker.GetValidation(queuedID)
	require.NoError(t, err)
	require.Equal(t, StakerStatusExited, getStatusRes.Status)

	// WithdrawDelegation
	receipt, _, err = staker.WithdrawDelegation(delegationID).Send().WithSigner(stargate).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	withdrawDelegationEvents, err := staker.FilterDelegationWithdrawn(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, withdrawDelegationEvents, 1)

	// GetDelegation after withdrawal
	delegationStake, err = staker.GetDelegation(delegationID)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).Cmp(delegationStake.Stake), 0)

	// GetDelegatorsRewards
	rewards, err := staker.GetDelegatorsRewards(validator.Address, 1)
	require.NoError(t, err)
	require.Equal(t, 0, big.NewInt(0).Cmp(rewards))

	// Issuance
	issuance, err := staker.Issuance("best")
	require.NoError(t, err)
	require.Equal(t, 1, issuance.Sign())

	best := node.Chain().Repo().BestBlockSummary()
	state := node.Chain().Stater().NewState(best.Root())
	energy := builtin.Energy.Native(state, best.Header.Timestamp())
	stakerNative := builtin.Staker.Native(state)

	rewards, err = energy.CalculateRewards(stakerNative)
	require.NoError(t, err)
	require.Equal(t, rewards, issuance)

	// SetBeneficiary
	validator1 := bind.NewSigner(validators[0].PrivateKey)
	beneficiary := datagen.RandAddress()
	receipt, _, err = staker.SetBeneficiary(firstID, beneficiary).
		Send().
		WithSigner(validator1).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	DebugRevert(t, receipt, staker.SetBeneficiary(queuedID, beneficiary))

	beneficiaryEvents, err := staker.FilterBeneficiarySet(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, beneficiaryEvents, 1)
	require.Equal(t, validator1.Address(), beneficiaryEvents[0].Validator)
	require.Equal(t, beneficiary, beneficiaryEvents[0].Beneficiary)
}

func TestStaker_Raw_MinStake_And_EmptyQueues(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	s, err := NewStaker(client)
	require.NoError(t, err)

	raw := s.Raw()
	require.NotNil(t, raw)
	require.Equal(t, builtin.Staker.Address, *raw.Address())
	_, ok := raw.ABI().Methods["totalStake"]
	require.True(t, ok)

	expected := new(big.Int).Mul(big.NewInt(1e18), big.NewInt(25_000_000))
	require.Equal(t, expected, MinStake())

	_, _, err = s.FirstActive()
	require.Error(t, err)
	_, _, err = s.FirstQueued()
	require.Error(t, err)
}

func TestValidatorStake_Exists_False(t *testing.T) {
	v := Validation{Endorser: thor.Address{}, Status: StakerStatusUnknown}
	require.False(t, v.Exists())
}

func TestStaker_Filter_EventNotFound(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	// build staker with wrong ABI so events are unknown
	badContract, err := bind.NewContract(client, builtin.Energy.RawABI(), &builtin.Staker.Address)
	require.NoError(t, err)
	bad := &Staker{contract: badContract}

	cases := []struct {
		name string
		call func() error
	}{
		{"ValidationQueued", func() error { _, err := bad.FilterValidatorQueued(nil, nil, logdb.ASC); return err }},
		{"ValidationSignaledExit", func() error { _, err := bad.FilterValidationSignaledExit(nil, nil, logdb.ASC); return err }},
		{"DelegationAdded", func() error { _, err := bad.FilterDelegationAdded(nil, nil, logdb.ASC); return err }},
		{"DelegationSignaledExit", func() error { _, err := bad.FilterDelegationSignaledExit(nil, nil, logdb.ASC); return err }},
		{"DelegationWithdrawn", func() error { _, err := bad.FilterDelegationWithdrawn(nil, nil, logdb.ASC); return err }},
		{"StakeIncreased", func() error { _, err := bad.FilterStakeIncreased(nil, nil, logdb.ASC); return err }},
		{"StakeDecreased", func() error { _, err := bad.FilterStakeDecreased(nil, nil, logdb.ASC); return err }},
		{"BeneficiarySet", func() error { _, err := bad.FilterBeneficiarySet(nil, nil, logdb.ASC); return err }},
		{"ValidationWithdrawn", func() error { _, err := bad.FilterValidationWithdrawn(nil, nil, logdb.ASC); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.call())
		})
	}
}

func TestStaker_NegativeMatrix_MethodNotFound(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	// Wrong ABI so all method lookups fail
	badContract, err := bind.NewContract(client, builtin.Energy.RawABI(), &builtin.Staker.Address)
	require.NoError(t, err)
	bad := &Staker{contract: badContract}

	nodeAddr := genesis.DevAccounts()[0].Address

	reads := []struct {
		name string
		run  func() error
	}{
		{"TotalStake", func() error { _, _, err := bad.TotalStake(); return err }},
		{"QueuedStake", func() error { _, err := bad.QueuedStake(); return err }},
		{"GetValidation", func() error { _, err := bad.GetValidation(nodeAddr); return err }},
		{"GetValidationPeriodDetails", func() error { _, err := bad.GetValidationPeriodDetails(nodeAddr); return err }},
		{"GetWithdrawable", func() error { _, err := bad.GetWithdrawable(nodeAddr); return err }},
		{"GetDelegatorsRewards", func() error { _, err := bad.GetDelegatorsRewards(nodeAddr, 1); return err }},
		{"GetDelegation", func() error { _, err := bad.GetDelegation(big.NewInt(1)); return err }},
		{"GetDelegationPeriodDetails", func() error { _, err := bad.GetDelegationPeriodDetails(big.NewInt(1)); return err }},
		{"GetValidationTotals", func() error { _, err := bad.GetValidationTotals(nodeAddr); return err }},
		{"GetValidationsNum", func() error { _, _, err := bad.GetValidationsNum(); return err }},
		{"Issuance", func() error { _, err := bad.Issuance("best"); return err }},
	}

	for _, tc := range reads {
		t.Run("WrongABI_"+tc.name, func(t *testing.T) {
			require.Error(t, tc.run())
		})
	}

	clauses := []struct {
		name string
		run  func() error
	}{
		{"AddValidation", func() error { _, err := bad.AddValidation(nodeAddr, MinStake(), 1).Clause(); return err }},
		{"SignalExit", func() error { _, err := bad.SignalExit(nodeAddr).Clause(); return err }},
		{"WithdrawStake", func() error { _, err := bad.WithdrawStake(nodeAddr).Clause(); return err }},
		{"DecreaseStake", func() error { _, err := bad.DecreaseStake(nodeAddr, big.NewInt(1)).Clause(); return err }},
		{"IncreaseStake", func() error { _, err := bad.IncreaseStake(nodeAddr, big.NewInt(1)).Clause(); return err }},
		{"SetBeneficiary", func() error { _, err := bad.SetBeneficiary(nodeAddr, nodeAddr).Clause(); return err }},
		{"AddDelegation", func() error { _, err := bad.AddDelegation(nodeAddr, MinStake(), 1).Clause(); return err }},
		{"SignalDelegationExit", func() error { _, err := bad.SignalDelegationExit(big.NewInt(1)).Clause(); return err }},
		{"WithdrawDelegation", func() error { _, err := bad.WithdrawDelegation(big.NewInt(1)).Clause(); return err }},
	}

	for _, tc := range clauses {
		t.Run("WrongABI_Clause_"+tc.name, func(t *testing.T) {
			require.Error(t, tc.run())
		})
	}
}

func TestStaker_BadRevision_Reads(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	s, err := NewStaker(client)
	require.NoError(t, err)

	addr := genesis.DevAccounts()[0].Address

	_, _, err = s.Revision("bad").TotalStake()
	require.Error(t, err)
	_, err = s.Revision("bad").QueuedStake()
	require.Error(t, err)
	_, err = s.Revision("bad").GetValidation(addr)
	require.Error(t, err)
	require.Error(t, err)
	_, err = s.Revision("bad").GetValidationPeriodDetails(addr)
	require.Error(t, err)
	_, err = s.Revision("bad").GetWithdrawable(addr)
	require.Error(t, err)
	_, _, err = s.Revision("bad").GetValidationsNum()
	require.Error(t, err)
	_, err = s.Issuance("bad")
	require.Error(t, err)
}

func TestStaker_Next_NoNext(t *testing.T) {
	minStakingPeriod := uint32(360) * 24 * 7

	node, client := newTestNode(t, false)
	defer node.Stop()

	staker, err := NewStaker(client)
	require.NoError(t, err)
	authority, err := NewAuthority(client)
	require.NoError(t, err)
	params, err := NewParams(client)
	require.NoError(t, err)

	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)
	stargate := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)

	for _, acc := range genesis.DevAccounts()[1:] {
		method := authority.Add(acc.Address, acc.Address, datagen.RandomHash())
		receipt, _, err := method.Send().WithSigner(executor).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
		require.NoError(t, err)
		DebugRevert(t, receipt, method)
	}

	_, _, err = params.Set(thor.KeyMaxBlockProposers, big.NewInt(3)).Send().WithSigner(executor).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	_, _, err = params.Set(thor.KeyDelegatorContractAddress, new(big.Int).SetBytes(stargate.Address().Bytes())).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	minStake := MinStake()
	valAcc := genesis.DevAccounts()[4]
	method := staker.AddValidation(valAcc.Address, minStake, minStakingPeriod)
	receipt, _, err := method.Send().WithSigner(bind.NewSigner(valAcc.PrivateKey)).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	DebugRevert(t, receipt, method)

	queuedEvents, err := staker.FilterValidatorQueued(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, queuedEvents, 1)
	queuedID := queuedEvents[0].Node
	_, _, err = staker.Next(queuedID)
	require.Error(t, err)
}
