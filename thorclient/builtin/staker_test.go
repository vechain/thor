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
	"github.com/vechain/thor/v2/builtin/solidity"
	builtinStaker "github.com/vechain/thor/v2/builtin/staker"
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
	if _, _, err := params.Set(thor.KeyStargateContractAddress, new(big.Int).SetBytes(stargate.Address().Bytes())).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t)); err != nil {
		t.Fatal(err)
	}

	// add validators - trigger PoS activation
	minStake := MinStake()

	builtinStaker.EpochLength = solidity.NewConfigVariable("epoch-length", 1)
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
	getStakeRes, err := staker.GetValidatorStake(firstID)
	require.NoError(t, err)
	require.False(t, getStakeRes.Address.IsZero())
	require.False(t, getStakeRes.Endorsor.IsZero())
	require.Equal(t, getStakeRes.Stake, minStake)
	require.Equal(t, getStakeRes.Weight, minStake)
	require.Equal(t, getStakeRes.QueuedStake.String(), big.NewInt(0).String())

	// GetStatus
	getStatusRes, err := staker.GetValidatorStatus(firstID)
	require.NoError(t, err)
	require.Equal(t, firstID, getStatusRes.Address)
	require.Equal(t, StakerStatusActive, getStatusRes.Status)
	require.True(t, getStatusRes.Online)
	require.True(t, getStakeRes.Exists(*getStatusRes))

	// GetPeriodDetails
	getPeriodDetailsRes, err := staker.GetValidatorPeriodDetails(firstID)
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
	require.True(t, firstActive.Exists(*getStatusRes))
	require.Equal(t, minStake, firstActive.Stake)
	require.Equal(t, minStake, firstActive.Weight)
	require.False(t, firstActive.Endorsor.IsZero())

	// Next
	next, id, err := staker.Next(firstID)
	require.NoError(t, err)
	require.False(t, id.IsZero())
	nextStatus, err := staker.GetValidatorStatus(next.Address)
	require.True(t, next.Exists(*nextStatus))
	require.NoError(t, err)
	require.Equal(t, StakerStatusActive, nextStatus.Status)
	require.Equal(t, minStake, next.Stake)
	require.Equal(t, minStake, next.Weight)
	require.False(t, next.Endorsor.IsZero())

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

	queuedEvents, err := staker.FilterValidatorQueued(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, queuedEvents, 1)
	require.Equal(t, validator.Address, queuedEvents[0].Endorsor)
	require.Equal(t, minStake, queuedEvents[0].Stake)
	queuedID := queuedEvents[0].Node

	// FirstQueued
	firstQueued, id, err := staker.FirstQueued()
	require.NoError(t, err)
	require.False(t, id.IsZero())
	firstQueuedStatus, err := staker.GetValidatorStatus(firstQueued.Address)
	require.NoError(t, err)
	require.True(t, firstQueued.Exists(*firstQueuedStatus))
	require.Equal(t, 0, firstQueued.Stake.Sign())
	require.Equal(t, StakerStatusQueued, firstQueuedStatus.Status)
	require.False(t, firstQueued.Endorsor.IsZero())

	// TotalQueued
	queuedStake, queuedWeight, err := staker.QueuedStake()
	require.NoError(t, err)
	stake := big.NewInt(0).Mul(big.NewInt(1e18), big.NewInt(25))
	stake = big.NewInt(0).Mul(stake, big.NewInt(1e6))
	require.Equal(t, stake, queuedStake)
	require.Equal(t, minStake, queuedWeight)

	builtinStaker.EpochLength = solidity.NewConfigVariable("epoch-length", 180)

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
	delegationStake, err := staker.GetDelegationStake(delegationID)
	require.NoError(t, err)
	require.Equal(t, minStake, delegationStake.Stake)
	require.Equal(t, uint8(100), delegationStake.Multiplier)
	require.Equal(t, queuedID, delegationStake.Validator)

	// GetDelegationPeriodDetails
	delegationPeriodDetails, err := staker.GetDelegationPeriodDetails(delegationID)
	require.NoError(t, err)
	require.Equal(t, uint32(1), delegationPeriodDetails.StartPeriod)
	require.Equal(t, uint32(math.MaxUint32), delegationPeriodDetails.EndPeriod)
	require.False(t, delegationPeriodDetails.Locked)

	// GetValidatorsTotals
	validationTotals, err := staker.GetValidationTotals(firstID)
	require.NoError(t, err)

	require.Equal(t, minStake, validationTotals.TotalLockedStake)
	require.Equal(t, minStake, validationTotals.TotalLockedWeight)
	require.Equal(t, big.NewInt(0).String(), validationTotals.TotalQueuedStake.String())
	require.Equal(t, big.NewInt(0).String(), validationTotals.TotalQueuedWeight.String())
	require.Equal(t, big.NewInt(0).String(), validationTotals.TotalExitingStake.String())
	require.Equal(t, big.NewInt(0).String(), validationTotals.TotalExitingWeight.String())

	// GetValidatorsNum
	active, queued, err := staker.GetValidatorsNum()
	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), active)
	require.Equal(t, big.NewInt(1), queued)

	// UpdateDelegationAutoRenew - Enable AutoRenew
	receipt, _, err = staker.SignalDelegationExit(delegationID).
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	ExpectRevert(t, receipt, staker.SignalDelegationExit(delegationID), "delegation has not started yet, funds can be withdrawn")

	// Withdraw
	receipt, _, err = staker.WithdrawStake(queuedID).Send().WithSigner(validatorKey).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	withdrawEvents, err := staker.FilterValidationWithdrawn(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, withdrawEvents, 1)

	getStatusRes, err = staker.GetValidatorStatus(queuedID)
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
	delegationStake, err = staker.GetDelegationStake(delegationID)
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
	require.Equal(t, validator1.Address(), beneficiaryEvents[0].Endorsor)
}
