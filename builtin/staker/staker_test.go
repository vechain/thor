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
	assertBigInts(ts.t, expectedVET, locked, "locked VET mismatch")
	assertBigInts(ts.t, expectedWeight, weight, "locked weight mismatch")
	return ts
}

func (ts *TestSequence) AssertQueuedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	queued, weight, err := ts.staker.QueuedStake()
	assert.NoError(ts.t, err, "failed to get queued VET")
	assertBigInts(ts.t, expectedVET, queued, "queued VET mismatch")
	assertBigInts(ts.t, expectedWeight, weight, "queued weight mismatch")

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
	endorser, master thor.Address,
	period uint32,
	stake *big.Int,
) *TestSequence {
	err := ts.staker.AddValidation(endorser, master, period, stake)
	assert.NoError(ts.t, err, "failed to add validator %s with endorser %s", master.String(), endorser.String())
	return ts
}

func (ts *TestSequence) SignalExit(validator, endorser thor.Address) *TestSequence {
	err := ts.staker.SignalExit(validator, endorser)
	assert.NoError(ts.t, err, "failed to signal exit for validator %s with endorser %s", validator.String(), endorser.String())
	return ts
}

func (ts *TestSequence) IncreaseStake(
	addr thor.Address,
	endorser thor.Address,
	amount *big.Int,
) *TestSequence {
	err := ts.staker.IncreaseStake(addr, endorser, amount)
	assert.NoError(ts.t, err, "failed to increase stake for validator %s by %s: %v", addr.String(), amount.String(), err)
	return ts
}

func (ts *TestSequence) DecreaseStake(
	addr thor.Address,
	endorser thor.Address,
	amount *big.Int,
) *TestSequence {
	err := ts.staker.DecreaseStake(addr, endorser, amount)
	assert.NoError(ts.t, err, "failed to decrease stake for validator %s by %s: %v", addr.String(), amount.String(), err)
	return ts
}

func (ts *TestSequence) WithdrawStake(endorser, master thor.Address, block uint32, expectedOut *big.Int) *TestSequence {
	amount, err := ts.staker.WithdrawStake(endorser, master, block)
	assert.NoError(ts.t, err, "failed to withdraw stake for validator %s with endorser %s at block %d: %v", master.String(), endorser.String(), block, err)
	assertBigInts(ts.t, expectedOut, amount, "withdrawn amount mismatch")
	return ts
}

func (ts *TestSequence) SetBeneficiary(
	validator thor.Address,
	endorser thor.Address,
	beneficiary thor.Address,
) *TestSequence {
	err := ts.staker.SetBeneficiary(validator, endorser, beneficiary)
	assert.NoError(ts.t, err, "failed to set beneficiary for validator %s with endorser %s: %v", validator.String(), endorser.String(), err)
	return ts
}

func (ts *TestSequence) AssertWithdrawable(
	master thor.Address,
	block uint32,
	expectedWithdrawable *big.Int,
) *TestSequence {
	withdrawable, err := ts.staker.GetWithdrawable(master, block)
	assert.NoError(ts.t, err, "failed to get withdrawable amount for validator %s at block %d: %v", master.String(), block, err)
	assertBigInts(ts.t, expectedWithdrawable, withdrawable, "withdrawable amount mismatch for validator")
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
	assertBigInts(ts.t, expectedOut, amount, "withdrawn delegation amount mismatch")
	return ts
}

func (ts *TestSequence) AssertDelegatorRewards(
	validationID thor.Address,
	period uint32,
	expectedReward *big.Int,
) *TestSequence {
	reward, err := ts.staker.GetDelegatorRewards(validationID, period)
	assert.NoError(ts.t, err, "failed to get rewards for validator %s at period %d: %v", validationID.String(), period, err)
	assertBigInts(ts.t, expectedReward, reward, "delegator rewards mismatch")
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

// AssertTotals for the contract. If any fields not set, it asserts they are zero.
func (ts *TestSequence) AssertTotals(validationID thor.Address, expected *validation.Totals) *TestSequence {
	totals, err := ts.staker.GetValidationTotals(validationID)
	assert.NoError(ts.t, err, "failed to get totals for validator %s", validationID.String())

	// exiting
	assertBigInts(ts.t, expected.TotalExitingStake, totals.TotalExitingStake, "total exiting stake mismatch")
	assertBigInts(ts.t, expected.TotalExitingWeight, totals.TotalExitingWeight, "total exiting weight mismatch")

	// locked
	assertBigInts(ts.t, expected.TotalLockedStake, totals.TotalLockedStake, "total locked stake mismatch")
	assertBigInts(ts.t, expected.TotalLockedWeight, totals.TotalLockedWeight, "total locked weight mismatch")

	// queued
	assertBigInts(ts.t, expected.TotalQueuedStake, totals.TotalQueuedStake, "total queued stake mismatch")
	assertBigInts(ts.t, expected.TotalQueuedWeight, totals.TotalQueuedWeight, "total queued weight mismatch")

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
	_, err := ts.staker.Housekeep(block)
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
	validator, err := staker.GetValidation(addr)
	require.NoError(t, err, "failed to get validator %s", addr.String())
	return &ValidationAssertions{staker: staker, addr: addr, validator: validator, t: t}
}

func (va *ValidationAssertions) Status(expected validation.Status) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.Status, "validator %s status mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) Weight(expected *big.Int) *ValidationAssertions {
	assertBigInts(va.t, expected, va.validator.Weight, "validator weight mismatch")
	return va
}

func (va *ValidationAssertions) LockedVET(expected *big.Int) *ValidationAssertions {
	assertBigInts(va.t, expected, va.validator.LockedVET, "validator locked VET mismatch")
	return va
}

func (va *ValidationAssertions) QueuedVET(expected *big.Int) *ValidationAssertions {
	assertBigInts(va.t, expected, va.validator.QueuedVET, "validator queued VET mismatch")
	return va
}

func (va *ValidationAssertions) CooldownVET(expected *big.Int) *ValidationAssertions {
	assertBigInts(va.t, expected, va.validator.CooldownVET, "validator cooldown VET mismatch")
	return va
}

func (va *ValidationAssertions) WithdrawableVET(expected *big.Int) *ValidationAssertions {
	assertBigInts(va.t, expected, va.validator.WithdrawableVET, "validator withdrawable VET mismatch")
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
	assertBigInts(va.t, expected, va.validator.PendingUnlockVET, "validator pending unlock VET mismatch")
	return va
}

func (va *ValidationAssertions) IsEmpty(expected bool) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.IsEmpty(), "validator %s empty state mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) Rewards(period uint32, expected *big.Int) *ValidationAssertions {
	reward, err := va.staker.GetDelegatorRewards(va.addr, period)
	assert.NoError(va.t, err, "failed to get rewards for validator %s at period %d", va.addr.String(), period)
	assertBigInts(va.t, expected, reward, "validator rewards mismatch")
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
	assertBigInts(aa.t, expected, aa.aggregation.PendingVET, "aggregation PendingVET mismatch")
	return aa
}

func (aa *AggregationAssertions) PendingWeight(expected *big.Int) *AggregationAssertions {
	assertBigInts(aa.t, expected, aa.aggregation.PendingWeight, "aggregation PendingWeight mismatch")
	return aa
}

func (aa *AggregationAssertions) LockedVET(expected *big.Int) *AggregationAssertions {
	assertBigInts(aa.t, expected, aa.aggregation.LockedVET, "aggregation LockedVET mismatch")
	return aa
}

func (aa *AggregationAssertions) LockedWeight(expected *big.Int) *AggregationAssertions {
	assertBigInts(aa.t, expected, aa.aggregation.LockedWeight, "aggregation LockedWeight mismatch")
	return aa
}

func (aa *AggregationAssertions) ExitingVET(expected *big.Int) *AggregationAssertions {
	assertBigInts(aa.t, expected, aa.aggregation.ExitingVET, "aggregation ExitingVET mismatch")
	return aa
}

func (aa *AggregationAssertions) ExitingWeight(expected *big.Int) *AggregationAssertions {
	assertBigInts(aa.t, expected, aa.aggregation.ExitingWeight, "aggregation ExitingWeight mismatch")
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
	assertBigInts(da.t, expected, da.delegation.Stake, "delegation stake mismatch")
	return da
}

func (da *DelegationAssertions) Weight(expected *big.Int) *DelegationAssertions {
	assertBigInts(da.t, expected, da.delegation.WeightedStake().Weight(), "delegation weight mismatch")
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

func assertBigInts(t *testing.T, expected, actual *big.Int, msg string) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		assert.True(t, actual.Sign() == 0, "%s: expected 0, got %s", msg, actual.String())
		return
	}
	if actual == nil {
		assert.True(t, expected.Sign() == 0, "%s: expected %s, got 0", msg, expected.String())
		return
	}

	assert.True(t, expected.Cmp(actual) == 0, "%s: expected %s, got %s", msg, expected.String(), actual.String())
}
