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

type TestFunc func(t *testing.T)

type TestSequence struct {
	staker *Staker

	funcs []TestFunc
}

func NewSequence(staker *Staker) *TestSequence {
	return &TestSequence{funcs: make([]TestFunc, 0), staker: staker}
}

func (ts *TestSequence) AddFunc(f TestFunc) *TestSequence {
	ts.funcs = append(ts.funcs, f)
	return ts
}

func (ts *TestSequence) AssertActive(active bool) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		isActive, err := ts.staker.IsPoSActive()
		assert.NoError(t, err, "failed to check PoS active state")
		assert.Equal(t, active, isActive, "PoS active state mismatch")
	})
}

func (ts *TestSequence) AssertLockedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		locked, weight, err := ts.staker.LockedVET()
		assert.NoError(t, err, "failed to get locked VET")
		if expectedVET != nil {
			assert.Equal(t, 0, expectedVET.Cmp(locked), "locked VET mismatch")
		}
		if expectedWeight != nil {
			assert.Equal(t, 0, expectedWeight.Cmp(weight), "locked weight mismatch")
		}
	})
}

func (ts *TestSequence) AssertQueuedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		queued, weight, err := ts.staker.QueuedStake()
		assert.NoError(t, err, "failed to get queued VET")
		if expectedVET != nil {
			assert.Equal(t, 0, expectedVET.Cmp(queued), "queued VET mismatch")
		}
		if expectedWeight != nil {
			assert.Equal(t, 0, expectedWeight.Cmp(weight), "queued weight mismatch")
		}
	})
}

func (ts *TestSequence) AssertFirstActive(expectedAddr thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		firstActive, err := ts.staker.FirstActive()
		assert.NoError(t, err, "failed to get first active validator")
		assert.Equal(t, expectedAddr, *firstActive, "first active validator mismatch")
	})
}

func (ts *TestSequence) AssertFirstQueued(expectedAddr thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		firstQueued, err := ts.staker.FirstQueued()
		assert.NoError(t, err, "failed to get first queued validator")
		assert.Equal(t, expectedAddr, *firstQueued, "first queued validator mismatch")
	})
}

func (ts *TestSequence) AssertQueueSize(expectedSize int64) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		size, err := ts.staker.QueuedGroupSize()
		assert.NoError(t, err, "failed to get queue size")
		assert.Equal(t, expectedSize, size.Int64(), "queue size mismatch")
	})
}

func (ts *TestSequence) AssertLeaderGroupSize(expectedSize int64) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		size, err := ts.staker.LeaderGroupSize()
		assert.NoError(t, err, "failed to get leader group size")
		assert.Equal(t, expectedSize, size.Int64(), "leader group size mismatch")
	})
}

func (ts *TestSequence) AssertNext(prev thor.Address, expected thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		next, err := ts.staker.Next(prev)
		assert.NoError(t, err, "failed to get next validator after %s", prev.String())
		assert.Equal(t, expected, next, "next validator mismatch after %s", prev.String())
	})
}

func (ts *TestSequence) AddValidator(
	endorsor, master thor.Address,
	period uint32,
	stake *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.AddValidator(endorsor, master, period, stake)
		assert.NoError(t, err, "failed to add validator %s with endorsor %s", master.String(), endorsor.String())
	})
}

func (ts *TestSequence) SignalExit(endorsor, master thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.SignalExit(endorsor, master)
		assert.NoError(t, err, "failed to signal exit for validator %s with endorsor %s", master.String(), endorsor.String())
	})
}

func (ts *TestSequence) IncreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.IncreaseStake(addr, endorsor, amount)
		assert.NoError(t, err, "failed to increase stake for validator %s by %s: %v", addr.String(), amount.String(), err)
	})
}

func (ts *TestSequence) DecreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.DecreaseStake(addr, endorsor, amount)
		assert.NoError(t, err, "failed to decrease stake for validator %s by %s: %v", addr.String(), amount.String(), err)
	})
}

func (ts *TestSequence) WithdrawStake(endorsor, master thor.Address, block uint32, expectedOut *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		amount, err := ts.staker.WithdrawStake(endorsor, master, block)
		assert.NoError(t, err, "failed to withdraw stake for validator %s with endorsor %s at block %d: %v", master.String(), endorsor.String(), block, err)
		assert.Equal(
			t,
			0,
			amount.Cmp(expectedOut),
			"withdrawn amount mismatch for validator %s with endorsor %s at block %d",
			master.String(),
			endorsor.String(),
			block,
		)
	})
}

func (ts *TestSequence) AssertWithdrawable(
	master thor.Address,
	block uint32,
	expectedWithdrawable *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		withdrawable, err := ts.staker.GetWithdrawable(master, block)
		assert.NoError(t, err, "failed to get withdrawable amount for validator %s at block %d: %v", master.String(), block, err)
		assert.Equal(t, expectedWithdrawable, withdrawable, "withdrawable amount mismatch for validator %s", master.String())
	})
}

func (ts *TestSequence) SetOnline(id thor.Address, online bool) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		_, err := ts.staker.SetOnline(id, online)
		assert.NoError(t, err, "failed to set online status for validator %s: %v", id.String(), err)
	})
}

func (ts *TestSequence) AddDelegation(
	master thor.Address,
	amount *big.Int,
	multiplier uint8,
	id *thor.Bytes32,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		delegationID, err := ts.staker.AddDelegation(master, amount, multiplier)
		assert.NoError(
			t,
			err,
			"failed to add delegation for validator %s with amount %s and multiplier %d: %v",
			master.String(),
			amount.String(),
			multiplier,
			err,
		)
		*id = delegationID
	})
}

func (ts *TestSequence) AssertHasDelegations(node thor.Address, expected bool) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		hasDelegations, err := ts.staker.HasDelegations(node)
		assert.NoError(t, err, "failed to check delegations for validator %s: %v", node.String(), err)
		assert.Equal(t, expected, hasDelegations, "delegation presence mismatch for validator %s", node.String())
	})
}

func (ts *TestSequence) SignalDelegationExit(delegationID thor.Bytes32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		assert.NoError(t, ts.staker.SignalDelegationExit(delegationID))
	})
}

func (ts *TestSequence) WithdrawDelegation(delegationID thor.Bytes32, expectedOut *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		amount, err := ts.staker.WithdrawDelegation(delegationID)
		assert.NoError(t, err, "failed to withdraw delegation %s: %v", delegationID.String(), err)
		assert.Equal(t, expectedOut, amount, "withdrawn amount mismatch for delegation %s", delegationID.String())
	})
}

func (ts *TestSequence) AssertDelegatorRewards(
	validationID thor.Address,
	period uint32,
	expectedReward *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		reward, err := ts.staker.GetDelegatorRewards(validationID, period)
		assert.NoError(t, err, "failed to get rewards for validator %s at period %d: %v", validationID.String(), period, err)
		assert.Equal(t, expectedReward, reward, "reward mismatch for validator %s at period %d", validationID.String(), period)
	})
}

func (ts *TestSequence) AssertCompletedPeriods(
	validationID thor.Address,
	expectedPeriods uint32,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		periods, err := ts.staker.GetCompletedPeriods(validationID)
		assert.NoError(t, err, "failed to get completed periods for validator %s: %v", validationID.String(), err)
		assert.Equal(t, expectedPeriods, periods, "completed periods mismatch for validator %s", validationID.String())
	})
}

func (ts *TestSequence) ActivateNext(block uint32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		mbp, err := ts.staker.params.Get(thor.KeyMaxBlockProposers)
		assert.NoError(t, err, "failed to get max block proposers")
		_, err = ts.staker.ActivateNextValidator(block, mbp)
		assert.NoError(t, err, "failed to activate next validator at block %d", block)
	})
}

func (ts *TestSequence) Housekeep(block uint32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		_, _, err := ts.staker.Housekeep(block)
		assert.NoError(t, err, "failed to perform housekeeping at block %d", block)
	})
}

func (ts *TestSequence) Transition(block uint32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		_, err := ts.staker.Transition(block)
		assert.NoError(t, err, "failed to transition at block %d", block)
	})
}

func (ts *TestSequence) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		assert.NoError(t, ts.staker.IncreaseDelegatorsReward(node, reward))
	})
}

func (ts *TestSequence) Run(t *testing.T) {
	for _, f := range ts.funcs {
		f(t)
	}
}

type ValidatorAssertions struct {
	staker    *Staker
	addr      thor.Address
	validator *validation.Validation
	t         *testing.T
}

func AssertValidator(t *testing.T, staker *Staker, addr thor.Address) *ValidatorAssertions {
	validator, err := staker.Get(addr)
	require.NoError(t, err, "failed to get validator %s", addr.String())
	return &ValidatorAssertions{staker: staker, addr: addr, validator: validator, t: t}
}

func (va *ValidatorAssertions) Status(expected validation.Status) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.Status, "validator %s status mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) Weight(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.Weight), "validator %s weight mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) LockedVET(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.LockedVET), "validator %s locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) PendingLocked(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.PendingLocked), "validator %s pending locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) CooldownVET(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.CooldownVET), "validator %s cooldown VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) WithdrawableVET(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, 0, expected.Cmp(va.validator.WithdrawableVET), "validator %s withdrawable VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) Period(expected uint32) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.Period, "validator %s period mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) CompletedPeriods(expected uint32) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.CompleteIterations, "validator %s completed periods mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) NextPeriodDecrease(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.NextPeriodDecrease, "validator %s next period decrease mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) IsEmpty(expected bool) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.IsEmpty(), "validator %s empty state mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) Rewards(period uint32, expected *big.Int) *ValidatorAssertions {
	reward, err := va.staker.GetDelegatorRewards(va.addr, period)
	assert.NoError(va.t, err, "failed to get rewards for validator %s at period %d", va.addr.String(), period)
	assert.Equal(va.t, expected, reward, "validator %s rewards mismatch for period %d", va.addr.String(), period)
	return va
}

type AggregationAssertions struct {
	staker       *Staker
	validationID thor.Address
	aggregation  *aggregation.Aggregation
	t            *testing.T
}

func AssertAggregation(t *testing.T, staker *Staker, validationID thor.Address) *AggregationAssertions {
	agg, err := staker.aggregationService.GetAggregation(validationID)
	require.NoError(t, err, "failed to get aggregation for validator %s", validationID.String())
	return &AggregationAssertions{staker: staker, validationID: validationID, aggregation: agg, t: t}
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

func (aa *AggregationAssertions) WithdrawableVET(expected *big.Int) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.WithdrawableVET, "aggregation for validator %s withdrawable VET mismatch", aa.validationID.String())
	return aa
}

type DelegationAssertions struct {
	staker       *Staker
	delegationID thor.Bytes32
	t            *testing.T
	delegation   *delegation.Delegation
	validation   *validation.Validation
}

func AssertDelegation(t *testing.T, staker *Staker, delegationID thor.Bytes32) *DelegationAssertions {
	delegation, validation, err := staker.GetDelegation(delegationID)
	require.NoError(t, err, "failed to get delegation %s", delegationID.String())
	return &DelegationAssertions{staker: staker, delegationID: delegationID, t: t, delegation: delegation, validation: validation}
}

func (da *DelegationAssertions) ValidationID(expected thor.Address) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.ValidationID, "delegation %s validation ID mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) Stake(expected *big.Int) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.Stake, "delegation %s stake mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) Weight(expected *big.Int) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.CalcWeight(), "delegation %s weight mismatch", da.delegationID.String())
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

func (da *DelegationAssertions) LastIteration(expected uint32) *DelegationAssertions {
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
