// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/thor"
)

type TestFunc func(t *testing.T)

type TestSequence struct {
	staker *Staker

	funcs []TestFunc
	mu    sync.Mutex
}

func NewSequence(staker *Staker) *TestSequence {
	return &TestSequence{funcs: make([]TestFunc, 0), staker: staker}
}

func (ts *TestSequence) AddFunc(f TestFunc) *TestSequence {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.funcs = append(ts.funcs, f)
	return ts
}

func (ts *TestSequence) AssertActive(active bool) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		isActive, err := ts.staker.IsPoSActive()
		if err != nil {
			t.Fatalf("failed to check if PoS is active: %v", err)
		}
		assert.Equal(t, active, isActive, "PoS active state mismatch")
	})
}

func (ts *TestSequence) AssertLockedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		locked, weight, err := ts.staker.LockedVET()
		if err != nil {
			t.Fatalf("failed to get locked VET: %v", err)
		}
		assert.Equal(t, expectedVET, locked, "locked VET mismatch")
		assert.Equal(t, expectedWeight, weight, "locked weight mismatch")
	})
}

func (ts *TestSequence) AssertQueuedVET(expectedVET, expectedWeight *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		queued, weight, err := ts.staker.QueuedStake()
		if err != nil {
			t.Fatalf("failed to get queued VET: %v", err)
		}
		assert.Equal(t, expectedVET, queued, "queued VET mismatch")
		assert.Equal(t, expectedWeight, weight, "queued weight mismatch")
	})
}

func (ts *TestSequence) AssertFirstActive(expectedAddr thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		firstActive, err := ts.staker.FirstActive()
		if err != nil {
			t.Fatalf("failed to get first active validator: %v", err)
		}
		assert.Equal(t, expectedAddr, *firstActive, "first active validator mismatch")
	})
}

func (ts *TestSequence) AssertFirstQueued(expectedAddr thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		firstQueued, err := ts.staker.FirstQueued()
		if err != nil {
			t.Fatalf("failed to get first queued validator: %v", err)
		}
		assert.Equal(t, expectedAddr, *firstQueued, "first queued validator mismatch")
	})
}

func (ts *TestSequence) AssertQueueSize(expectedSize int64) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		size, err := ts.staker.QueuedGroupSize()
		if err != nil {
			t.Fatalf("failed to get queue size: %v", err)
		}
		assert.Equal(t, expectedSize, size.Int64(), "queue size mismatch")
		t.Logf("queue size: %d", size)
	})
}

func (ts *TestSequence) AssertLeaderGroupSize(expectedSize int64) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		size, err := ts.staker.LeaderGroupSize()
		if err != nil {
			t.Fatalf("failed to get leader group size: %v", err)
		}
		assert.Equal(t, expectedSize, size.Int64(), "leader group size mismatch")
		t.Logf("leader group size: %d", size)
	})
}

func (ts *TestSequence) AssertNext(prev thor.Address, expected thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		next, err := ts.staker.Next(prev)
		if err != nil {
			t.Fatalf("failed to get next validator after %s: %v", prev.String(), err)
		}
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
		if err != nil {
			t.Fatalf("failed to add validator %s: %v", master.String(), err)
		}
		t.Logf("added validator %s", master.String())
	})
}

func (ts *TestSequence) SignalExit(endorsor, master thor.Address) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.SignalExit(endorsor, master)
		if err != nil {
			t.Fatalf("failed to signal exit for validator %s: %v", master, err)
		}
		t.Logf("exit signaled for validator %s", master.String())
	})
}

func (ts *TestSequence) IncreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.IncreaseStake(addr, endorsor, amount)
		if err != nil {
			t.Fatalf("failed to increase stake for validator %s: %v", addr, err)
		}
		t.Logf("increased stake for validator %s by %s", addr.String(), amount.String())
	})
}

func (ts *TestSequence) DecreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.DecreaseStake(addr, endorsor, amount)
		if err != nil {
			t.Fatalf("failed to decrease stake for validator %s: %v", addr, err)
		}
		t.Logf("decreased stake for validator %s by %s", addr.String(), amount.String())
	})
}

func (ts *TestSequence) WithdrawStake(endorsor, master thor.Address, block uint32, expectedOut *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		amount, err := ts.staker.WithdrawStake(endorsor, master, block)
		if err != nil {
			t.Fatalf("failed to withdraw from validator %s: %v", master, err)
		}
		t.Logf("withdrawn %s from validator %s", amount.String(), master.String())
		assert.Equal(t, expectedOut, amount, "withdrawn amount mismatch for validator %s", master.String())
	})
}

func (ts *TestSequence) AssertWithdrawable(
	master thor.Address,
	block uint32,
	expectedWithdrawable *big.Int,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		withdrawable, err := ts.staker.GetWithdrawable(master, block)
		if err != nil {
			t.Fatalf("failed to get withdrawable amount for validator %s: %v", master, err)
		}
		assert.Equal(t, expectedWithdrawable, withdrawable, "withdrawable amount mismatch for validator %s", master.String())
	})
}

func (ts *TestSequence) SetOnline(id thor.Address, online bool) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		_, err := ts.staker.SetOnline(id, online)
		if err != nil {
			t.Fatalf("failed to set online status for validator %s: %v", id.String(), err)
		}
		t.Logf("set online status for validator %s to %t", id.String(), online)
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
		if err != nil {
			t.Fatalf("failed to add delegation for validator %s: %v", master.String(), err)
		}
		t.Logf("added delegation %s for validator %s", delegationID.String(), master.String())
		*id = delegationID
	})
}

func (ts *TestSequence) AssertHasDelegations(node thor.Address, expected bool) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		hasDelegations, err := ts.staker.HasDelegations(node)
		if err != nil {
			t.Fatalf("failed to check delegations for validator %s: %v", node.String(), err)
		}
		assert.Equal(t, expected, hasDelegations, "delegation presence mismatch for validator %s", node.String())
		if expected {
			t.Logf("validator %s has delegations", node.String())
		} else {
			t.Logf("validator %s has no delegations", node.String())
		}
	})
}

func (ts *TestSequence) SignalDelegationExit(delegationID thor.Bytes32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.SignalDelegationExit(delegationID)
		if err != nil {
			t.Fatalf("failed to signal exit for delegation %s: %v", delegationID.String(), err)
		}
		t.Logf("exit signaled for delegation %s", delegationID.String())
	})
}

func (ts *TestSequence) WithdrawDelegation(delegationID thor.Bytes32, expectedOut *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		amount, err := ts.staker.WithdrawDelegation(delegationID)
		if err != nil {
			t.Fatalf("failed to withdraw from delegation %s: %v", delegationID.String(), err)
		}
		t.Logf("withdrawn %s from delegation %s", amount.String(), delegationID.String())
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
		if err != nil {
			t.Fatalf("failed to get rewards for validator %s at period %d: %v", validationID.String(), period, err)
		}
		assert.Equal(t, expectedReward, reward, "reward mismatch for validator %s at period %d", validationID.String(), period)
	})
}

func (ts *TestSequence) AssertCompletedPeriods(
	validationID thor.Address,
	expectedPeriods uint32,
) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		periods, err := ts.staker.GetCompletedPeriods(validationID)
		if err != nil {
			t.Fatalf("failed to get completed periods for validator %s: %v", validationID.String(), err)
		}
		assert.Equal(t, expectedPeriods, periods, "completed periods mismatch for validator %s", validationID.String())
	})
}

func (ts *TestSequence) ActivateNext(block uint32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		mbp, err := ts.staker.params.Get(thor.KeyMaxBlockProposers)
		if err != nil {
			t.Fatalf("failed to get max block proposers: %v", err)
		}
		addr, err := ts.staker.ActivateNextValidator(block, mbp)
		if err != nil {
			t.Fatalf("failed to activate next validator: %v", err)
		}
		t.Logf("activated next validator: %s", addr.String())
	})
}

func (ts *TestSequence) Housekeep(block uint32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		_, _, err := ts.staker.Housekeep(block)
		if err != nil {
			t.Fatalf("failed to housekeep at block %d: %v", block, err)
		}
		t.Logf("housekeeping completed at block %d", block)
	})
}

func (ts *TestSequence) Transition(block uint32) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		transitioned, err := ts.staker.Transition(block)
		if err != nil {
			t.Fatalf("failed to transition at block %d: %v", block, err)
		}
		t.Logf("transitioned at block %d: %v", block, transitioned)
	})
}

func (ts *TestSequence) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) *TestSequence {
	return ts.AddFunc(func(t *testing.T) {
		err := ts.staker.IncreaseDelegatorsReward(node, reward)
		if err != nil {
			t.Fatalf("failed to increase rewards for validator %s: %v", node, err)
		}
		t.Logf("increased rewards for validator %s by %s", node.String(), reward.String())
	})
}

func (ts *TestSequence) Run(t *testing.T) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for _, f := range ts.funcs {
		f(t)
	}

	t.Logf("All test functions executed successfully")
}

type ValidatorAssertions struct {
	staker    *Staker
	addr      thor.Address
	validator *Validation
	t         *testing.T
}

func AssertValidator(t *testing.T, staker *Staker, addr thor.Address) *ValidatorAssertions {
	validator, err := staker.Get(addr)
	require.NoError(t, err, "failed to get validator %s", addr.String())
	return &ValidatorAssertions{staker: staker, addr: addr, validator: validator, t: t}
}

func (va *ValidatorAssertions) Status(expected Status) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.Status, "validator %s status mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) Weight(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.Weight, "validator %s weight mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) LockedVET(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.LockedVET, "validator %s locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) PendingLocked(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.PendingLocked, "validator %s pending locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) CooldownVET(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.CooldownVET, "validator %s cooldown VET mismatch", va.addr.String())
	return va
}

func (va *ValidatorAssertions) WithdrawableVET(expected *big.Int) *ValidatorAssertions {
	assert.Equal(va.t, expected, va.validator.WithdrawableVET, "validator %s withdrawable VET mismatch", va.addr.String())
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
	delegation   *Delegation
	validation   *Validation
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
