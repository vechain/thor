// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/state"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/delegation"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

type TestSequence struct {
	staker *testStaker
	t      *testing.T
}

func newTestSequence(t *testing.T, staker *testStaker) *TestSequence {
	return &TestSequence{staker: staker, t: t}
}

func (ts *TestSequence) State() *state.State {
	return ts.staker.state
}

func (ts *TestSequence) Address() thor.Address {
	return ts.staker.Address()
}

func (ts *TestSequence) AssertActive(active bool) *TestSequence {
	isActive, err := ts.staker.IsPoSActive()
	assert.NoError(ts.t, err, "failed to check PoS active state")
	assert.Equal(ts.t, active, isActive, "PoS active state mismatch")
	return ts
}

func (ts *TestSequence) AssertLockedVET(expectedVET, expectedWeight uint64) *TestSequence {
	locked, weight, err := ts.staker.LockedStake()
	assert.NoError(ts.t, err, "failed to get locked VET")
	assert.Equal(ts.t, expectedVET, locked, "locked VET mismatch, got %d, expected %d", locked, expectedVET)
	assert.Equal(ts.t, expectedWeight, weight, "locked weight mismatch, got %d, expected %d", weight, expectedWeight)

	return ts
}

func (ts *TestSequence) AssertQueuedVET(expectedVET uint64) *TestSequence {
	queued, err := ts.staker.QueuedStake()
	assert.NoError(ts.t, err, "failed to get queued VET")
	assert.Equal(ts.t, expectedVET, queued, "queued VET mismatch, got %d, expected %d", queued, expectedVET)

	return ts
}

func (ts *TestSequence) AssertValidationNums(expectedActive, expectedQueued uint64) *TestSequence {
	active, queued, err := ts.staker.GetValidationsNum()
	assert.NoError(ts.t, err, "failed to get validation numbers")
	assert.Equal(ts.t, expectedActive, active, "active validators count mismatch, got %d, expected %d", active, expectedActive)
	assert.Equal(ts.t, expectedQueued, queued, "queued validators count mismatch, got %d, expected %d", queued, expectedQueued)
	return ts
}

func (ts *TestSequence) AssertFirstActive(expectedAddr thor.Address) *TestSequence {
	firstActive, err := ts.staker.FirstActive()
	assert.NoError(ts.t, err, "failed to get first active validator")
	assert.Equal(ts.t, expectedAddr, firstActive, "first active validator mismatch")
	return ts
}

func (ts *TestSequence) AssertFirstQueued(expectedAddr thor.Address) *TestSequence {
	firstQueued, err := ts.staker.FirstQueued()
	assert.NoError(ts.t, err, "failed to get first queued validator")
	assert.Equal(ts.t, expectedAddr, firstQueued, "first queued validator mismatch")
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

func (ts *TestSequence) FirstActive() (thor.Address, *validation.Validation) {
	first, err := ts.staker.FirstActive()
	assert.NoError(ts.t, err)
	val, err := ts.staker.GetValidation(first)
	assert.NoError(ts.t, err)
	return first, val
}

func (ts *TestSequence) FirstQueued() (thor.Address, *validation.Validation) {
	first, err := ts.staker.FirstQueued()
	assert.NoError(ts.t, err)
	val, err := ts.staker.GetValidation(first)
	assert.NoError(ts.t, err)
	return first, val
}

func (ts *TestSequence) Next(prev thor.Address) (thor.Address, *validation.Validation) {
	first, err := ts.staker.Next(prev)
	assert.NoError(ts.t, err)
	val, err := ts.staker.GetValidation(first)
	assert.NoError(ts.t, err)
	return first, val
}

func (ts *TestSequence) GetValidation(addr thor.Address) *validation.Validation {
	val, err := ts.staker.GetValidation(addr)
	assert.NoError(ts.t, err, "failed to get validator %s", addr.String())
	return val
}

func (ts *TestSequence) GetAggregation(addr thor.Address) *aggregation.Aggregation {
	agg, err := ts.staker.aggregationService.GetAggregation(addr)
	assert.NoError(ts.t, err, "failed to get aggregation for validator %s", addr.String())
	return agg
}

func (ts *TestSequence) GetDelegation(delegationID *big.Int) *delegation.Delegation {
	del, _, err := ts.staker.GetDelegation(delegationID)
	assert.NoError(ts.t, err, "failed to get delegation %s", delegationID.String())
	return del
}

func (ts *TestSequence) AddValidationErrors(
	validator, endorser thor.Address,
	period uint32,
	stake uint64,
	errMsg string,
) *TestSequence {
	err := ts.staker.AddValidation(validator, endorser, period, stake)
	assert.NotNil(ts.t, err, "expected error when adding validator %s with endorser %s", validator.String(), endorser.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when adding validator %s with endorser %s", validator.String(), endorser.String())
	return ts
}

func (ts *TestSequence) AddValidation(
	validator, endorser thor.Address,
	period uint32,
	stake uint64,
) *TestSequence {
	err := ts.staker.AddValidation(validator, endorser, period, stake)
	assert.NoError(ts.t, err, "failed to add validator %s with endorser %s", validator.String(), endorser.String())
	return ts
}

func (ts *TestSequence) UpdateContractBalance(amount uint64) *TestSequence {
	addr := ts.staker.Address()
	current, err := ts.staker.state.GetBalance(addr)
	assert.NoError(ts.t, err, "failed to get contract balance")
	if current == nil {
		current = big.NewInt(0)
	}
	newBalance := new(big.Int).Add(current, big.NewInt(int64(amount)))
	assert.NoError(ts.t, ts.staker.state.SetBalance(addr, newBalance))
	return ts
}

func (ts *TestSequence) SignalExit(validator, endorser thor.Address, currentBlock uint32) *TestSequence {
	err := ts.staker.SignalExit(validator, endorser, currentBlock)
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

func (ts *TestSequence) IncreaseStakeErrors(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
	errMsg string,
) *TestSequence {
	err := ts.staker.IncreaseStake(addr, endorser, amount)
	assert.NotNil(ts.t, err, "expected error when increasing stake for validator %s by %d", addr.String(), amount)
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when increasing stake for validator %s by %d", addr.String(), amount)
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

func (ts *TestSequence) DecreaseStakeErrors(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
	errMsg string,
) *TestSequence {
	err := ts.staker.DecreaseStake(addr, endorser, amount)
	assert.NotNil(ts.t, err, "expected error when decreasing stake for validator %s by %d", addr.String(), amount)
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when decreasing stake for validator %s by 1", addr.String())
	return ts
}

func (ts *TestSequence) WithdrawStake(validator, endorser thor.Address, block uint32, expectedOut uint64) *TestSequence {
	amount, err := ts.staker.WithdrawStake(validator, endorser, block)
	assert.NoError(ts.t, err, "failed to withdraw stake for validator %s with endorser %s at block %d: %v", validator.String(), endorser.String(), block, err)
	assert.Equal(
		ts.t,
		amount, expectedOut,
		"withdrawn amount mismatch for validator %s with endorser %s at block %d",
		validator.String(),
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

func (ts *TestSequence) SetBeneficiaryErrors(
	validator thor.Address,
	endorser thor.Address,
	beneficiary thor.Address,
	errMsg string,
) *TestSequence {
	err := ts.staker.SetBeneficiary(validator, endorser, beneficiary)
	assert.NotNil(ts.t, err, "expected error when setting beneficiary for validator %s with endorser %s", validator.String(), endorser.String())
	assert.ErrorContains(
		ts.t,
		err,
		errMsg,
		"expected error message when setting beneficiary for validator %s with endorser %s",
		validator.String(),
		endorser.String(),
	)
	return ts
}

func (ts *TestSequence) AssertWithdrawable(
	validator thor.Address,
	block uint32,
	expectedWithdrawable uint64,
) *TestSequence {
	withdrawable, err := ts.staker.GetWithdrawable(validator, block)
	assert.NoError(ts.t, err, "failed to get withdrawable amount for validator %s at block %d: %v", validator.String(), block, err)
	assert.Equal(ts.t, expectedWithdrawable, withdrawable, "withdrawable amount mismatch for validator %s", validator.String())
	return ts
}

func (ts *TestSequence) SetOnline(id thor.Address, blockNum uint32, online bool) *TestSequence {
	err := ts.staker.SetOnline(id, blockNum, online)
	assert.NoError(ts.t, err, "failed to set online status for validator %s: %v", id.String(), err)
	return ts
}

func (ts *TestSequence) AddDelegation(
	validator thor.Address,
	amount uint64,
	multiplier uint8,
	currentBlock uint32,
) *big.Int {
	delegationID, err := ts.staker.AddDelegation(validator, amount, multiplier, currentBlock)
	assert.NoError(
		ts.t,
		err,
		"failed to add delegation for validator %s with amount %d and multiplier %d: %v",
		validator.String(),
		amount,
		multiplier,
		err,
	)
	return delegationID
}

func (ts *TestSequence) AddDelegationErrors(
	validator thor.Address,
	amount uint64,
	multiplier uint8,
	currentBlock uint32,
	errMsg string,
) *TestSequence {
	_, err := ts.staker.AddDelegation(validator, amount, multiplier, currentBlock)
	assert.NotNil(ts.t, err, "expected error when adding delegation for validator %s with amount %d and multiplier %d", validator.String(), amount, multiplier)
	assert.ErrorContains(
		ts.t,
		err,
		errMsg,
		"expected error message when adding delegation for validator %s with amount %d and multiplier %d",
		validator.String(),
		amount,
		multiplier,
	)
	return ts
}

func (ts *TestSequence) AssertHasDelegations(node thor.Address, expected bool) *TestSequence {
	hasDelegations, err := ts.staker.HasDelegations(node)
	assert.NoError(ts.t, err, "failed to check delegations for validator %s: %v", node.String(), err)
	assert.Equal(ts.t, expected, hasDelegations, "delegation presence mismatch for validator %s", node.String())
	return ts
}

func (ts *TestSequence) SignalDelegationExit(delegationID *big.Int, currentBlock uint32) *TestSequence {
	assert.NoError(ts.t, ts.staker.SignalDelegationExit(delegationID, currentBlock))
	return ts
}

func (ts *TestSequence) WithdrawDelegation(delegationID *big.Int, expectedOut uint64, currentBlock uint32) *TestSequence {
	amount, err := ts.staker.WithdrawDelegation(delegationID, currentBlock)
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
	currentBlock uint32,
) *TestSequence {
	val, err := ts.staker.GetValidation(validationID)
	assert.NotNil(ts.t, val, "validation %s not found", validationID.String())
	assert.NoError(ts.t, err, "failed to get validation %s", validationID.String())
	periods, err := val.CompletedIterations(currentBlock)
	assert.NoError(ts.t, err, "failed to get completed periods for validator %s: %v", validationID.String(), err)
	assert.Equal(ts.t, expectedPeriods, periods, "completed periods mismatch for validator %s", validationID.String())
	return ts
}

func (ts *TestSequence) AssertTotals(validationID thor.Address, expected *validation.Totals) *TestSequence {
	totals, err := ts.staker.GetValidationTotals(validationID)
	assert.NoError(ts.t, err, "failed to get totals for validator %s", validationID.String())

	// exiting
	assert.Equal(
		ts.t,
		expected.TotalExitingStake,
		totals.TotalExitingStake,
		"total exiting stake mismatch for validator %s, expected=%d, got=%d",
		validationID.String(),
		expected.TotalExitingStake,
		totals.TotalExitingStake,
	)

	// locked
	assert.Equal(
		ts.t,
		expected.TotalLockedStake,
		totals.TotalLockedStake,
		"total locked stake mismatch for validator %s, expected=%d, got=%d",
		validationID.String(),
		expected.TotalLockedStake,
		totals.TotalLockedStake,
	)
	assert.Equal(
		ts.t,
		expected.TotalLockedWeight,
		totals.TotalLockedWeight,
		"total locked weight mismatch for validator %s, expected=%d, got=%d",
		validationID.String(),
		expected.TotalLockedWeight,
		totals.TotalLockedWeight,
	)

	// queued
	assert.Equal(
		ts.t,
		expected.TotalQueuedStake,
		totals.TotalQueuedStake,
		"total queued stake mismatch for validator %s, expected=%d, got=%d",
		validationID.String(),
		expected.TotalQueuedStake,
		totals.TotalQueuedStake,
	)
	assert.Equal(
		ts.t,
		expected.NextPeriodWeight,
		totals.NextPeriodWeight,
		"next period weight mismatch for validator %s, expected=%d, got=%d",
		validationID.String(),
		expected.NextPeriodWeight,
		totals.NextPeriodWeight,
	)

	return ts
}

func (ts *TestSequence) AssertGlobalWithdrawable(expected uint64) *TestSequence {
	withdrawable, err := ts.staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(ts.t, err, "failed to get global withdrawable")

	assert.Equal(ts.t, expected, withdrawable, "total withdrawable mismatch")

	return ts
}

func (ts *TestSequence) AssertGlobalCooldown(expected uint64) *TestSequence {
	cooldown, err := ts.staker.globalStatsService.GetCooldownStake()
	assert.NoError(ts.t, err, "failed to get global cooldown")

	assert.Equal(ts.t, expected, cooldown, "total cooldown mismatch")

	return ts
}

func (ts *TestSequence) ActivateNext(block uint32) *TestSequence {
	mbp, err := ts.staker.params.Get(thor.KeyMaxBlockProposers)
	assert.NoError(ts.t, err, "failed to get max block proposers")
	_, err = ts.staker.activateNextValidation(block, mbp.Uint64())
	assert.NoError(ts.t, err, "failed to activate next validator at block %d", block)
	return ts
}

func (ts *TestSequence) ActivateNextErrors(block uint32, errMsg string) *TestSequence {
	mbp, err := ts.staker.params.Get(thor.KeyMaxBlockProposers)
	assert.NoError(ts.t, err, "failed to get max block proposers")
	_, err = ts.staker.activateNextValidation(block, mbp.Uint64())
	assert.NotNil(ts.t, err, "expected error when activating next validator at block %d", block)
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when activating next validator at block %d", block)
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

func (ts *TestSequence) IncreaseDelegatorsReward(node thor.Address, reward *big.Int, currentBlock uint32) *TestSequence {
	assert.NoError(ts.t, ts.staker.IncreaseDelegatorsReward(node, reward, currentBlock))
	return ts
}

// ExitValidator manually exits a validator, skipping the housekeeping part
func (ts *TestSequence) ExitValidator(node thor.Address) *TestSequence {
	exit, err := ts.staker.validationService.ExitValidator(node)
	assert.NoError(ts.t, err, "failed to exit validator %s", node.String())
	aggExit, err := ts.staker.aggregationService.Exit(node)
	assert.NoError(ts.t, err, "failed to exit aggregation for validator %s", node.String())
	assert.NoError(ts.t, ts.staker.globalStatsService.ApplyExit(exit, aggExit))
	return ts
}

func (ts *TestSequence) ExitValidatorErrors(node thor.Address, errMsg string) *TestSequence {
	_, err := ts.staker.validationService.ExitValidator(node)
	assert.NotNil(ts.t, err, "expected error when exiting validator %s", node.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when exiting validator %s", node.String())
	return ts
}

func (ts *TestSequence) AssertValidation(addr thor.Address) *ValidationAssertions {
	return assertValidation(ts.t, ts, addr)
}

func (ts *TestSequence) AssertAggregation(validationID thor.Address) *AggregationAssertions {
	return assertAggregation(ts.t, ts.staker, validationID)
}

func (ts *TestSequence) AssertDelegation(delegationID *big.Int, currentBlock uint32) *DelegationAssertions {
	return assertDelegation(ts.t, ts.staker, delegationID, currentBlock)
}

type ValidationAssertions struct {
	addr      thor.Address
	validator *validation.Validation
	t         *testing.T
}

func assertValidation(t *testing.T, test *TestSequence, addr thor.Address) *ValidationAssertions {
	validator, err := test.staker.GetValidation(addr)
	require.NoError(t, err, "failed to get validator %s", addr.String())
	return &ValidationAssertions{addr: addr, validator: validator, t: t}
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

func (va *ValidationAssertions) PendingUnlockVET(expected uint64) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.PendingUnlockVET, "validator %s next period decrease mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) IsEmpty(expected bool) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator == nil, "validator %s empty state mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) Beneficiary(expected thor.Address) *ValidationAssertions {
	if expected.IsZero() {
		assert.Nil(va.t, va.validator.Beneficiary, "validator %s beneficiary mismatch", va.addr.String())
	} else {
		assert.Equal(va.t, expected, *va.validator.Beneficiary, "validator %s beneficiary mismatch", va.addr.String())
	}
	return va
}

func (va *ValidationAssertions) OfflineBlock(expected *uint32) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.OfflineBlock, "validator %s offline block mismatch", va.addr.String())
	return va
}

func (va *ValidationAssertions) ExitBlock(expected *uint32) *ValidationAssertions {
	assert.Equal(va.t, expected, va.validator.ExitBlock, "validator %s exit block mismatch", va.addr.String())
	return va
}

type AggregationAssertions struct {
	validationID thor.Address
	aggregation  *aggregation.Aggregation
	t            *testing.T
}

func assertAggregation(t *testing.T, staker *testStaker, validationID thor.Address) *AggregationAssertions {
	agg, err := staker.aggregationService.GetAggregation(validationID)
	require.NoError(t, err, "failed to get aggregation for validator %s", validationID.String())
	return &AggregationAssertions{validationID: validationID, aggregation: agg, t: t}
}

func (aa *AggregationAssertions) PendingVET(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.Pending.VET, "aggregation for validator %s pending VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) PendingWeight(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.Pending.Weight, "aggregation for validator %s pending weight mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) LockedVET(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.Locked.VET, "aggregation for validator %s locked VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) LockedWeight(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.Locked.Weight, "aggregation for validator %s locked weight mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) ExitingVET(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.Exiting.VET, "aggregation for validator %s exiting VET mismatch", aa.validationID.String())
	return aa
}

func (aa *AggregationAssertions) ExitingWeight(expected uint64) *AggregationAssertions {
	assert.Equal(aa.t, expected, aa.aggregation.Exiting.Weight, "aggregation for validator %s exiting weight mismatch", aa.validationID.String())
	return aa
}

type DelegationAssertions struct {
	delegationID *big.Int
	t            *testing.T
	delegation   *delegation.Delegation
	validation   *validation.Validation
	currentBlock uint32
}

func assertDelegation(t *testing.T, staker *testStaker, delegationID *big.Int, currentBlock uint32) *DelegationAssertions {
	delegation, validation, err := staker.GetDelegation(delegationID)
	require.NoError(t, err, "failed to get delegation %s", delegationID.String())
	return &DelegationAssertions{delegationID: delegationID, t: t, delegation: delegation, validation: validation, currentBlock: currentBlock}
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
		started, err := da.delegation.Started(da.validation, da.currentBlock)
		assert.NoError(da.t, err)
		ended, err := da.delegation.Ended(da.validation, da.currentBlock)
		assert.NoError(da.t, err)
		assert.True(da.t, started, "delegation %s locked state mismatch", da.delegationID.String())
		assert.False(da.t, ended, "delegation %s ended state mismatch", da.delegationID.String())
	} else {
		started, err := da.delegation.Started(da.validation, da.currentBlock)
		assert.NoError(da.t, err)
		ended, err := da.delegation.Ended(da.validation, da.currentBlock)
		assert.NoError(da.t, err)
		if started && !ended {
			da.t.Fatalf("delegation %s is expected to be not locked, but it is", da.delegationID.String())
		}
	}
	return da
}

func (da *DelegationAssertions) IsStarted(expected bool) *DelegationAssertions {
	started, err := da.delegation.Started(da.validation, da.currentBlock)
	assert.NoError(da.t, err)
	assert.Equal(da.t, expected, started, "delegation %s started state mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) IsFinished(expected bool) *DelegationAssertions {
	ended, err := da.delegation.Ended(da.validation, da.currentBlock)
	assert.NoError(da.t, err)
	assert.Equal(da.t, expected, ended, "delegation %s finished state mismatch", da.delegationID.String())
	return da
}
