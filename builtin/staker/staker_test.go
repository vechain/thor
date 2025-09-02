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

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/delegation"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
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
	locked, weight, err := ts.staker.LockedStake()
	assert.NoError(ts.t, err, "failed to get locked VET")
	if expectedVET != nil {
		assert.Equal(ts.t, expectedVET, locked, "locked VET mismatch")
	}
	if expectedWeight != nil {
		assert.Equal(ts.t, expectedWeight, weight, "locked weight mismatch")
	}
	return ts
}

func (ts *TestSequence) AssertQueuedVET(expectedVET *big.Int) *TestSequence {
	queued, err := ts.staker.QueuedStake()
	assert.NoError(ts.t, err, "failed to get queued VET")
	if expectedVET != nil {
		assert.Equal(ts.t, expectedVET, queued, "queued VET mismatch")
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

func (ts *TestSequence) AssertQueueSize(expectedSize uint64) *TestSequence {
	size, err := ts.staker.QueuedGroupSize()
	assert.NoError(ts.t, err, "failed to get queue size")
	assert.Equal(ts.t, expectedSize, size, "queue size mismatch")
	return ts
}

func (ts *TestSequence) AssertLeaderGroupSize(expectedSize uint64) *TestSequence {
	size, err := ts.staker.LeaderGroupSize()
	assert.NoError(ts.t, err, "failed to get leader group size")
	assert.Equal(ts.t, expectedSize, size, "leader group size mismatch")
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
	stake uint64,
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
	amount uint64,
) *TestSequence {
	err := ts.staker.IncreaseStake(addr, endorser, amount)
	assert.NoError(ts.t, err, "failed to increase stake for validator %s by %d: %v", addr.String(), amount, err)
	return ts
}

func (ts *TestSequence) DecreaseStake(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
) *TestSequence {
	err := ts.staker.DecreaseStake(addr, endorser, amount)
	assert.NoError(ts.t, err, "failed to decrease stake for validator %s by %d: %v", addr.String(), amount, err)
	return ts
}

func (ts *TestSequence) WithdrawStake(endorser, master thor.Address, block uint32, expectedOut uint64) *TestSequence {
	amount, err := ts.staker.WithdrawStake(endorser, master, block)
	assert.NoError(ts.t, err, "failed to withdraw stake for validator %s with endorser %s at block %d: %v", master.String(), endorser.String(), block, err)
	assert.Equal(
		ts.t,
		amount, expectedOut,
		"withdrawn amount mismatch for validator %s with endorser %s at block %d",
		master.String(),
		endorser.String(),
		block,
	)
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
	amount uint64,
	multiplier uint8,
	idSetter *big.Int,
) *TestSequence {
	delegationID, err := ts.staker.AddDelegation(master, amount, multiplier)
	assert.NoError(
		ts.t,
		err,
		"failed to add delegation for validator %s with amount %d and multiplier %d: %v",
		master.String(),
		amount,
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

func (ts *TestSequence) WithdrawDelegation(delegationID *big.Int, expectedOut uint64) *TestSequence {
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
	assert.Equal(ts.t, expected.TotalExitingStake, totals.TotalExitingStake, "total exiting stake mismatch for validator %s", validationID.String())

	// locked
	assert.Equal(ts.t, expected.TotalLockedStake, totals.TotalLockedStake, "total locked stake mismatch for validator %s", validationID.String())
	assert.Equal(ts.t, expected.TotalLockedWeight, totals.TotalLockedWeight, "total locked weight mismatch for validator %s", validationID.String())

	// queued
	assert.Equal(ts.t, expected.TotalQueuedStake, totals.TotalQueuedStake, "total queued stake mismatch for validator %s", validationID.String())

	assert.Equal(ts.t, expected.NextPeriodWeight, totals.NextPeriodWeight, "next period weight mismatch for validator %s", validationID.String())

	return ts
}

func (ts *TestSequence) ActivateNext(block uint32) *TestSequence {
	mbp, err := ts.staker.params.Get(thor.KeyMaxBlockProposers)
	assert.NoError(ts.t, err, "failed to get max block proposers")
	_, err = ts.staker.activateNextValidation(block, mbp.Uint64())
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

func (va *ValidationAssertions) Weight(expected uint64) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.Weight, "validator %s weight mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) LockedVET(expected uint64) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.LockedVET, "validator %s locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) QueuedVET(expected uint64) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.QueuedVET, "validator %s pending locked VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) CooldownVET(expected uint64) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.CooldownVET, "validator %s cooldown VET mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) WithdrawableVET(expected uint64) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.WithdrawableVET, "validator %s withdrawable VET mismatch", va.addr.String())
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

func (aa *AggregationAssertions) PendingVET(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.PendingVET, "aggregation for validator %s pending VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) PendingWeight(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.PendingWeight, "aggregation for validator %s pending weight mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) LockedVET(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.LockedVET, "aggregation for validator %s locked VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) LockedWeight(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.LockedWeight, "aggregation for validator %s locked weight mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) ExitingVET(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.ExitingVET, "aggregation for validator %s exiting VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) ExitingWeight(expected uint64) *AggregationAssertions {
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

func (da *DelegationAssertions) Stake(expected uint64) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.Stake, "delegation %s stake mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) Weight(expected uint64) *DelegationAssertions {
	assert.Equal(da.t, expected, da.delegation.WeightedStake().Weight, "delegation %s weight mismatch", da.delegationID.String())
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

func newTestStaker() *Staker {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	addr := thor.BytesToAddress([]byte("staker"))
	return New(addr, st, params.New(addr, st), nil)
}

func TestValidation_SignalExit_InvalidEndorser(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.SignalExit(id, thor.BytesToAddress([]byte("wrong")))
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidation_SignalExit_NotActive(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.SignalExit(id, end)
	assert.ErrorContains(t, err, "can't signal exit while not active")
}

func TestService_IncreaseStake_UnknownValidator(t *testing.T) {
	staker := newTestStaker()
	id := thor.BytesToAddress([]byte("unknown"))
	err := staker.IncreaseStake(id, id, 1)
	assert.ErrorContains(t, err, "validation does not exist")
}

func TestValidation_IncreaseStake_InvalidEndorser(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.IncreaseStake(id, thor.BytesToAddress([]byte("wrong")), 10)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidation_IncreaseStake_StatusExit(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	_, err := staker.WithdrawStake(id, end, 1)
	assert.NoError(t, err)

	err = staker.IncreaseStake(id, end, 5)
	assert.ErrorContains(t, err, "validator exited")
}

func TestValidation_IncreaseStake_ActiveHasExitBlock(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	val, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.SignalExit(id, end)
	assert.NoError(t, err)

	err = staker.IncreaseStake(id, end, 5)
	assert.ErrorContains(t, err, "validator has signaled exit, cannot increase stake")
}

func TestValidation_DecreaseStake_UnknownValidator(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("unknown"))
	err := staker.DecreaseStake(id, id, 1)
	assert.ErrorContains(t, err, "validation does not exist")
}

func TestValidation_DecreaseStake_InvalidEndorser(t *testing.T) {
	staker := newTestStaker()
	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.DecreaseStake(id, thor.BytesToAddress([]byte("wrong")), 1)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidation_DecreaseStake_StatusExit(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	val, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.SignalExit(id, end)
	assert.NoError(t, err)

	err = staker.DecreaseStake(id, end, 5)
	assert.ErrorContains(t, err, "validator has signaled exit, cannot decrease stake")
}

func TestValidation_DecreaseStake_ActiveHasExitBlock(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	val, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.SignalExit(id, end)
	assert.NoError(t, err)

	err = staker.DecreaseStake(id, end, 5)
	assert.ErrorContains(t, err, "validator has signaled exit, cannot decrease stake")
}

func TestValidation_DecreaseStake_ActiveTooLowNextPeriod(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET))

	err := staker.DecreaseStake(id, end, 100)
	assert.ErrorContains(t, err, "next period stake is lower than minimum stake")
}

func TestValidation_DecreaseStake_ActiveSuccess(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), MinStakeVET+100))

	val, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.DecreaseStake(id, end, 100)
	assert.NoError(t, err)

	v, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), v.PendingUnlockVET)
	assert.Equal(t, MinStakeVET+100, v.LockedVET)
}

func TestValidation_DecreaseStake_QueuedTooLowNextPeriod(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET))

	err := staker.DecreaseStake(id, end, 100)
	assert.ErrorContains(t, err, "next period stake is lower than minimum stake")
}

func TestValidation_DecreaseStake_QueuedSuccess(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET+100))

	assert.NoError(t, staker.DecreaseStake(id, end, 100))

	v, err := staker.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, MinStakeVET, v.QueuedVET)
	assert.Equal(t, uint64(100), v.WithdrawableVET)
}

func TestValidation_WithdrawStake_InvalidEndorser(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	endorsor := id
	assert.NoError(t, staker.AddValidation(id, endorsor, thor.LowStakingPeriod(), MinStakeVET))

	amt, err := staker.WithdrawStake(id, thor.BytesToAddress([]byte("wrong")), 0)
	assert.Equal(t, uint64(0), amt)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidationAdd_Error(t *testing.T) {
	staker := newTestStaker()

	id1 := thor.BytesToAddress([]byte("id1"))

	assert.ErrorContains(t, staker.AddValidation(id1, id1, uint32(1), MinStakeVET), "period is out of boundaries")
	assert.ErrorContains(t, staker.AddValidation(id1, id1, thor.LowStakingPeriod(), 0), "stake is out of range")
	assert.NoError(t, staker.AddValidation(id1, id1, thor.LowStakingPeriod(), MinStakeVET))
	assert.ErrorContains(t, staker.AddValidation(id1, id1, thor.LowStakingPeriod(), MinStakeVET), "validator already exists")
}

func TestValidation_SetBeneficiary_Error(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	endorsor := id
	assert.NoError(t, staker.AddValidation(id, endorsor, thor.LowStakingPeriod(), MinStakeVET))

	assert.ErrorContains(t, staker.SetBeneficiary(id, thor.BytesToAddress([]byte("wrong")), id), "endorser required")

	_, err := staker.WithdrawStake(id, id, 0)
	assert.NoError(t, err)

	assert.ErrorContains(t, staker.SetBeneficiary(id, id, id), "validator has exited or signaled exit, cannot set beneficiary")
}

func TestDelegation_Add_InputValidation(t *testing.T) {
	staker := newTestStaker()

	_, err := staker.AddDelegation(thor.Address{}, 1, 0)
	assert.ErrorContains(t, err, "multiplier cannot be 0")
}

func TestDelegation_SignalExit(t *testing.T) {
	staker := newTestStaker()

	v := thor.BytesToAddress([]byte("v"))
	assert.NoError(t, staker.AddValidation(v, v, thor.MediumStakingPeriod(), MinStakeVET))

	id, err := staker.AddDelegation(v, 3, 100)
	assert.NoError(t, err)

	val, err := staker.validationService.GetValidation(v)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	staker.validationService.ActivateValidator(v, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	_, _, err = staker.GetDelegation(id)
	assert.NoError(t, err)

	assert.NoError(t, staker.SignalDelegationExit(id))

	del2, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.NotNil(t, del2.LastIteration)
	assert.Equal(t, uint32(1), *del2.LastIteration)

	assert.ErrorContains(t, staker.SignalDelegationExit(id), "delegation is already signaled exit")
}

func TestDelegation_SignalExit_AlreadyWithdrawn(t *testing.T) {
	staker := newTestStaker()

	v := thor.BytesToAddress([]byte("v"))
	assert.NoError(t, staker.AddValidation(v, v, thor.MediumStakingPeriod(), MinStakeVET))

	id, err := staker.AddDelegation(v, 3, 100)
	assert.NoError(t, err)

	_, _, err = staker.GetDelegation(id)
	assert.NoError(t, err)
	amt, err := staker.WithdrawDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), amt)

	assert.ErrorContains(t, staker.SignalDelegationExit(id), "delegation has already been withdrawn")
}

func TestDelegation_SignalExit_Empty(t *testing.T) {
	staker := newTestStaker()

	assert.ErrorContains(t, staker.SignalDelegationExit(big.NewInt(2)), "delegation is empty")
}
