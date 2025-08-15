// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/delegation"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

type TestSequence struct {
	staker *Staker
	t      *testing.T
}

func newTestSequence(t *testing.T, staker *Staker) *TestSequence {
	return &TestSequence{staker: staker, t: t}
}

func (ts *TestSequence) AssertActive(active bool) *TestSequence {
	isActive, err := ts.staker.IsPoSActive()
	assert.NoError(ts.t, err, "failed to check PoS active state")
	assert.Equal(ts.t, active, isActive, "PoS active state mismatch")
	return ts
}

func (ts *TestSequence) AssertLockedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	locked, weight, err := ts.staker.LockedVET()
	assert.NoError(ts.t, err, "failed to get locked VET")
	if expectedVET != nil {
		assert.Equal(ts.t, 0, expectedVET.Cmp(locked), "locked VET mismatch")
	}
	if expectedWeight != nil {
		assert.Equal(ts.t, 0, expectedWeight.Cmp(weight), "locked weight mismatch")
	}
	return ts
}

func (ts *TestSequence) AssertQueuedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	queued, weight, err := ts.staker.QueuedStake()
	assert.NoError(ts.t, err, "failed to get queued VET")
	if expectedVET != nil {
		assert.Equal(ts.t, 0, expectedVET.Cmp(queued), "queued VET mismatch")
	}
	if expectedWeight != nil {
		assert.Equal(ts.t, 0, expectedWeight.Cmp(weight), "queued weight mismatch")
	}

	return ts
}

func (ts *TestSequence) AssertFirstActive(expectedAddr thor.Address) *TestSequence {
	firstActive, err := ts.staker.FirstActive()
	assert.NoError(ts.t, err, "failed to get first active validator")
	assert.Equal(ts.t, expectedAddr, *firstActive, "first active validator mismatch")
	return ts
}

func (ts *TestSequence) AssertFirstQueued(expectedAddr thor.Address) *TestSequence {
	firstQueued, err := ts.staker.FirstQueued()
	assert.NoError(ts.t, err, "failed to get first queued validator")
	assert.Equal(ts.t, expectedAddr, *firstQueued, "first queued validator mismatch")
	return ts
}

func (ts *TestSequence) AssertQueueSize(expectedSize int64) *TestSequence {
	size, err := ts.staker.QueuedGroupSize()
	assert.NoError(ts.t, err, "failed to get queue size")
	assert.Equal(ts.t, expectedSize, size.Int64(), "queue size mismatch")
	return ts
}

func (ts *TestSequence) AssertLeaderGroupSize(expectedSize int64) *TestSequence {
	size, err := ts.staker.LeaderGroupSize()
	assert.NoError(ts.t, err, "failed to get leader group size")
	assert.Equal(ts.t, expectedSize, size.Int64(), "leader group size mismatch")
	return ts
}

func (ts *TestSequence) AssertNext(prev thor.Address, expected thor.Address) *TestSequence {
	next, err := ts.staker.Next(prev)
	assert.NoError(ts.t, err, "failed to get next validator after %s", prev.String())
	assert.Equal(ts.t, expected, next, "next validator mismatch after %s", prev.String())
	return ts
}

func (ts *TestSequence) AddValidation(
	endorsor, master thor.Address,
	period uint32,
	stake *big.Int,
) *TestSequence {
	err := ts.staker.AddValidation(endorsor, master, period, stake)
	assert.NoError(ts.t, err, "failed to add validator %s with endorsor %s", master.String(), endorsor.String())
	return ts
}

func (ts *TestSequence) SignalExit(validator, endorsor thor.Address) *TestSequence {
	err := ts.staker.SignalExit(validator, endorsor)
	assert.NoError(ts.t, err, "failed to signal exit for validator %s with endorsor %s", validator.String(), endorsor.String())
	return ts
}

func (ts *TestSequence) IncreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	err := ts.staker.IncreaseStake(addr, endorsor, amount)
	assert.NoError(ts.t, err, "failed to increase stake for validator %s by %s: %v", addr.String(), amount.String(), err)
	return ts
}

func (ts *TestSequence) DecreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	err := ts.staker.DecreaseStake(addr, endorsor, amount)
	assert.NoError(ts.t, err, "failed to decrease stake for validator %s by %s: %v", addr.String(), amount.String(), err)
	return ts
}

func (ts *TestSequence) WithdrawStake(endorsor, master thor.Address, block uint32, expectedOut *big.Int) *TestSequence {
	amount, err := ts.staker.WithdrawStake(endorsor, master, block)
	assert.NoError(ts.t, err, "failed to withdraw stake for validator %s with endorsor %s at block %d: %v", master.String(), endorsor.String(), block, err)
	assert.Equal(
		ts.t,
		0,
		amount.Cmp(expectedOut),
		"withdrawn amount mismatch for validator %s with endorsor %s at block %d",
		master.String(),
		endorsor.String(),
		block,
	)
	return ts
}

func (ts *TestSequence) SetBeneficiary(
	validator thor.Address,
	endorsor thor.Address,
	beneficiary thor.Address,
) *TestSequence {
	err := ts.staker.SetBeneficiary(validator, endorsor, beneficiary)
	assert.NoError(ts.t, err, "failed to set beneficiary for validator %s with endorsor %s: %v", validator.String(), endorsor.String(), err)
	return ts
}

func (ts *TestSequence) AssertWithdrawable(
	master thor.Address,
	block uint32,
	expectedWithdrawable *big.Int,
) *TestSequence {
	withdrawable, err := ts.staker.GetWithdrawable(master, block)
	assert.NoError(ts.t, err, "failed to get withdrawable amount for validator %s at block %d: %v", master.String(), block, err)
	assert.Equal(ts.t, expectedWithdrawable, withdrawable, "withdrawable amount mismatch for validator %s", master.String())
	return ts
}

func (ts *TestSequence) SetOnline(id thor.Address, blockNum uint32, online bool) *TestSequence {
	err := ts.staker.SetOnline(id, blockNum, online)
	assert.NoError(ts.t, err, "failed to set online status for validator %s: %v", id.String(), err)
	return ts
}

func (ts *TestSequence) AddDelegation(
	master thor.Address,
	amount *big.Int,
	multiplier uint8,
	idSetter *big.Int,
) *TestSequence {
	delegationID, err := ts.staker.AddDelegation(master, amount, multiplier)
	assert.NoError(
		ts.t,
		err,
		"failed to add delegation for validator %s with amount %s and multiplier %d: %v",
		master.String(),
		amount.String(),
		multiplier,
		err,
	)
	idSetter.Set(delegationID)
	return ts
}

func (ts *TestSequence) AssertHasDelegations(node thor.Address, expected bool) *TestSequence {
	hasDelegations, err := ts.staker.HasDelegations(node)
	assert.NoError(ts.t, err, "failed to check delegations for validator %s: %v", node.String(), err)
	assert.Equal(ts.t, expected, hasDelegations, "delegation presence mismatch for validator %s", node.String())
	return ts
}

func (ts *TestSequence) SignalDelegationExit(delegationID *big.Int) *TestSequence {
	assert.NoError(ts.t, ts.staker.SignalDelegationExit(delegationID))
	return ts
}

func (ts *TestSequence) WithdrawDelegation(delegationID *big.Int, expectedOut *big.Int) *TestSequence {
	amount, err := ts.staker.WithdrawDelegation(delegationID)
	assert.NoError(ts.t, err, "failed to withdraw delegation %s: %v", delegationID.String(), err)
	assert.Equal(ts.t, expectedOut, amount, "withdrawn amount mismatch for delegation %s", delegationID.String())
	return ts
}

func (ts *TestSequence) AssertDelegatorRewards(
	validationID thor.Address,
	period uint32,
	expectedReward *big.Int,
) *TestSequence {
	reward, err := ts.staker.GetDelegatorRewards(validationID, period)
	assert.NoError(ts.t, err, "failed to get rewards for validator %s at period %d: %v", validationID.String(), period, err)
	assert.Equal(ts.t, expectedReward, reward, "reward mismatch for validator %s at period %d", validationID.String(), period)
	return ts
}

func (ts *TestSequence) AssertCompletedPeriods(
	validationID thor.Address,
	expectedPeriods uint32,
) *TestSequence {
	periods, err := ts.staker.GetCompletedPeriods(validationID)
	assert.NoError(ts.t, err, "failed to get completed periods for validator %s: %v", validationID.String(), err)
	assert.Equal(ts.t, expectedPeriods, periods, "completed periods mismatch for validator %s", validationID.String())
	return ts
}

func (ts *TestSequence) AssertTotals(validationID thor.Address, expected *validation.Totals) *TestSequence {
	totals, err := ts.staker.GetValidationTotals(validationID)
	assert.NoError(ts.t, err, "failed to get totals for validator %s", validationID.String())

	// exiting
	if expected.TotalExitingStake != nil {
		assert.Equal(ts.t, 0, expected.TotalExitingStake.Cmp(totals.TotalExitingStake), "total exiting stake mismatch for validator %s", validationID.String())
	}
	if expected.TotalExitingWeight != nil {
		assert.Equal(
			ts.t,
			0,
			expected.TotalExitingWeight.Cmp(totals.TotalExitingWeight),
			"total exiting weight mismatch for validator %s",
			validationID.String(),
		)
	}

	// locked
	if expected.TotalLockedStake != nil {
		assert.Equal(ts.t, 0, expected.TotalLockedStake.Cmp(totals.TotalLockedStake), "total locked stake mismatch for validator %s", validationID.String())
	}
	if expected.TotalLockedWeight != nil {
		assert.Equal(ts.t, 0, expected.TotalLockedWeight.Cmp(totals.TotalLockedWeight), "total locked weight mismatch for validator %s", validationID.String())
	}

	// queued
	if expected.TotalQueuedStake != nil {
		assert.Equal(ts.t, 0, expected.TotalQueuedStake.Cmp(totals.TotalQueuedStake), "total queued stake mismatch for validator %s", validationID.String())
	}
	if expected.TotalQueuedWeight != nil {
		assert.Equal(ts.t, 0, expected.TotalQueuedWeight.Cmp(totals.TotalQueuedWeight), "total queued weight mismatch for validator %s", validationID.String())
	}

	return ts
}

func (ts *TestSequence) ActivateNext(block uint32) *TestSequence {
	mbp, err := ts.staker.params.Get(thor.KeyMaxBlockProposers)
	assert.NoError(ts.t, err, "failed to get max block proposers")
	_, err = ts.staker.activateNextValidation(block, mbp)
	assert.NoError(ts.t, err, "failed to activate next validator at block %d", block)
	return ts
}

func (ts *TestSequence) Housekeep(block uint32) *TestSequence {
	_, _, err := ts.staker.Housekeep(block)
	assert.NoError(ts.t, err, "failed to perform housekeeping at block %d", block)
	return ts
}

func (ts *TestSequence) Transition(block uint32) *TestSequence {
	_, err := ts.staker.transition(block)
	assert.NoError(ts.t, err, "failed to transition at block %d", block)
	return ts
}

func (ts *TestSequence) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) *TestSequence {
	assert.NoError(ts.t, ts.staker.IncreaseDelegatorsReward(node, reward))
	return ts
}

type ValidationAssertions struct {
	staker    *Staker
	addr      thor.Address
	validator *validation.Validation
	t         *testing.T
}

func assertValidation(t *testing.T, staker *Staker, addr thor.Address) *ValidationAssertions {
	validator, err := staker.Get(addr)
	require.NoError(t, err, "failed to get validator %s", addr.String())
	return &ValidationAssertions{staker: staker, addr: addr, validator: validator, t: t}
}

func (va *ValidationAssertions) Status(expected validation.Status) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.Status, "validator %s status mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) Weight(expected *big.Int) *ValidationAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.Weight), "validator %s weight mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) LockedVET(expected *big.Int) *ValidationAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.LockedVET), "validator %s locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) QueuedVET(expected *big.Int) *ValidationAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.QueuedVET), "validator %s pending locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) CooldownVET(expected *big.Int) *ValidationAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.CooldownVET), "validator %s cooldown VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) WithdrawableVET(expected *big.Int) *ValidationAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.WithdrawableVET), "validator %s withdrawable VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) Period(expected uint32) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.Period, "validator %s period mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) CompletedPeriods(expected uint32) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.CompleteIterations, "validator %s completed periods mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) PendingUnlockVET(expected *big.Int) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.PendingUnlockVET, "validator %s next period decrease mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) IsEmpty(expected bool) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.IsEmpty(), "validator %s empty state mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) Rewards(period uint32, expected *big.Int) *ValidationAssertions {
	reward, err := va.staker.GetDelegatorRewards(va.addr, period)
	assert.NoError(va.t, err, "failed to get rewards for validator %s at period %d", va.addr.String(), period)
	assert.Equal(va.t, expected, reward, "validator %s rewards mismatch for period %d", va.addr.String(), period)
	return va
}

func (va *ValidationAssertions) Beneficiary(expected *thor.Address) *ValidationAssertions {
	if expected == nil {
		assert.Nil(va.t, va.validator.Beneficiary, "validator %s beneficiary mismatch", va.addr.String())
	} else {
		assert.Equal(va.t, *expected, *va.validator.Beneficiary, "validator %s beneficiary mismatch", va.addr.String())
	}
	return va
}

type AggregationAssertions struct {
	validationID thor.Address
	aggregation  *aggregation.Aggregation
	t            *testing.T
}

func assertAggregation(t *testing.T, staker *Staker, validationID thor.Address) *AggregationAssertions {
	agg, err := staker.aggregationService.GetAggregation(validationID)
	require.NoError(t, err, "failed to get aggregation for validator %s", validationID.String())
	return &AggregationAssertions{validationID: validationID, aggregation: agg, t: t}
}

func (aa *AggregationAssertions) PendingVET(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.PendingVET, "aggregation for validator %s pending VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) PendingWeight(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.PendingWeight, "aggregation for validator %s pending weight mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) LockedVET(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.LockedVET, "aggregation for validator %s locked VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) LockedWeight(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.LockedWeight, "aggregation for validator %s locked weight mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) ExitingVET(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.ExitingVET, "aggregation for validator %s exiting VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) ExitingWeight(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.ExitingWeight, "aggregation for validator %s exiting weight mismatch", aa.validationID.String())
	return aa
}

type DelegationAssertions struct {
	delegationID *big.Int
	t            *testing.T
	delegation   *delegation.Delegation
	validation   *validation.Validation
}

func assertDelegation(t *testing.T, staker *Staker, delegationID *big.Int) *DelegationAssertions {
	delegation, validation, err := staker.GetDelegation(delegationID)
	require.NoError(t, err, "failed to get delegation %s", delegationID.String())
	return &DelegationAssertions{delegationID: delegationID, t: t, delegation: delegation, validation: validation}
}

func (da *DelegationAssertions) Validation(expected thor.Address) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.Validation, "delegation %s validation ID mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) Stake(expected *big.Int) *DelegationAssertions {
	assert.Equal(da.t, 0, expected.Cmp(da.delegation.Stake), "delegation %s stake mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) Weight(expected *big.Int) *DelegationAssertions {
	assert.Equal(da.t, 0, expected.Cmp(da.delegation.WeightedStake().Weight()), "delegation %s weight mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) Multiplier(expected uint8) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.Multiplier, "delegation %s multiplier mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) FirstIteration(expected uint32) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.FirstIteration, "delegation %s first iteration mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) LastIteration(expected *uint32) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.LastIteration, "delegation %s last iteration mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) IsLocked(expected bool) *DelegationAssertions {
	if expected {
		assert.True(da.t, da.delegation.Started(da.validation), "delegation %s locked state mismatch", da.delegationID.String())
		assert.False(da.t, da.delegation.Ended(da.validation), "delegation %s ended state mismatch", da.delegationID.String())
	} else {
		started := da.delegation.Started(da.validation)
		ended := da.delegation.Ended(da.validation)
		if started && !ended {
			da.t.Fatalf("delegation %s is expected to be not locked, but it is", da.delegationID.String())
		}
	}
	return da
}

func (da *DelegationAssertions) IsStarted(expected bool) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.Started(da.validation), "delegation %s started state mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) IsFinished(expected bool) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.Ended(da.validation), "delegation %s finished state mismatch", da.delegationID.String())
	return da
}
