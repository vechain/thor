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
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

type StakerTest struct {
	*Staker
	t      *testing.T
	params *params.Params
	mbp    int64
}

func newTest(t *testing.T) *StakerTest {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	param := params.New(thor.BytesToAddress([]byte("params")), st)
	stakerAddr := thor.BytesToAddress([]byte("stkr"))

	require.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(101)))

	staker := New(stakerAddr, st, param, nil)

	return &StakerTest{
		Staker: staker,
		t:      t,
		params: param,
		mbp:    101,
	}
}

func (ts *StakerTest) State() *state.State {
	return ts.state
}

func (ts *StakerTest) Address() thor.Address {
	return ts.Staker.Address()
}

// Fill adds `amount` of random validators with random stakes and a medium staking period
func (ts *StakerTest) Fill(amount int) *StakerTest {
	for range amount {
		ts.AddValidation(datagen.RandAddress(), datagen.RandAddress(), thor.MediumStakingPeriod(), RandomStake())
	}
	return ts
}

// SetMBP sets the max block proposers to `mbp`
func (ts *StakerTest) SetMBP(mbp int64) *StakerTest {
	ts.mbp = mbp
	require.NoError(ts.t, ts.params.Set(thor.KeyMaxBlockProposers, big.NewInt(mbp)))
	return ts
}

// subContractVET subtracts `amount` VET from the contract balance
// Intended for internal use, but can be used by tests to simulate balance changes
func (ts *StakerTest) subContractVET(amount uint64) {
	balance, err := ts.state.GetBalance(ts.Staker.Address())
	require.NoError(ts.t, err, "failed to get contract balance")
	newBalance := big.NewInt(0).Sub(balance, ToWei(amount))
	require.NoError(ts.t, ts.state.SetBalance(ts.Staker.Address(), newBalance), "failed to set contract balance")
	newBalanceBytes := thor.BytesToBytes32(newBalance.Bytes())
	ts.state.SetStorage(ts.Staker.Address(), thor.Bytes32{}, newBalanceBytes)
}

// addContractVET adds `amount` VET to the contract balance
// Intended for internal use, but can be used by tests to simulate balance changes
func (ts *StakerTest) addContractVET(amount uint64) {
	balance, err := ts.state.GetBalance(ts.Staker.Address())
	require.NoError(ts.t, err, "failed to get contract balance")
	newBalance := big.NewInt(0).Add(balance, ToWei(amount))
	require.NoError(ts.t, ts.state.SetBalance(ts.Staker.Address(), newBalance), "failed to set contract balance")
	newBalanceBytes := thor.BytesToBytes32(newBalance.Bytes())
	ts.state.SetStorage(ts.Staker.Address(), thor.Bytes32{}, newBalanceBytes)
}

func (ts *StakerTest) AssertActive(active bool) *StakerTest {
	isActive, err := ts.IsPoSActive()
	assert.NoError(ts.t, err, "failed to check PoS active state")
	assert.Equal(ts.t, active, isActive, "PoS active state mismatch")
	return ts
}

func (ts *StakerTest) AssertLockedVET(expectedVET, expectedWeight uint64) *StakerTest {
	locked, weight, err := ts.Staker.LockedStake()
	assert.NoError(ts.t, err, "failed to get locked VET")
	assert.Equal(ts.t, expectedVET, locked, "locked VET mismatch, got %d, expected %d", locked, expectedVET)
	assert.Equal(ts.t, expectedWeight, weight, "locked weight mismatch, got %d, expected %d", weight, expectedWeight)

	return ts
}

func (ts *StakerTest) AssertQueuedVET(expectedVET uint64) *StakerTest {
	queued, err := ts.Staker.QueuedStake()
	assert.NoError(ts.t, err, "failed to get queued VET")
	assert.Equal(ts.t, expectedVET, queued, "queued VET mismatch, got %d, expected %d", queued, expectedVET)

	return ts
}

func (ts *StakerTest) AssertValidationNums(expectedActive, expectedQueued uint64) *StakerTest {
	active, queued, err := ts.GetValidationsNum()
	assert.NoError(ts.t, err, "failed to get validation numbers")
	assert.Equal(ts.t, expectedActive, active, "active validators count mismatch, got %d, expected %d", active, expectedActive)
	assert.Equal(ts.t, expectedQueued, queued, "queued validators count mismatch, got %d, expected %d", queued, expectedQueued)
	return ts
}

func (ts *StakerTest) AssertFirstActive(expectedAddr thor.Address) *StakerTest {
	firstActive, err := ts.Staker.FirstActive()
	assert.NoError(ts.t, err, "failed to get first active validator")
	assert.Equal(ts.t, expectedAddr, firstActive, "first active validator mismatch")
	return ts
}

func (ts *StakerTest) AssertFirstQueued(expectedAddr thor.Address) *StakerTest {
	firstQueued, err := ts.Staker.FirstQueued()
	assert.NoError(ts.t, err, "failed to get first queued validator")
	assert.Equal(ts.t, expectedAddr, firstQueued, "first queued validator mismatch")
	return ts
}

func (ts *StakerTest) AssertQueueSize(expectedSize uint64) *StakerTest {
	size, err := ts.QueuedGroupSize()
	assert.NoError(ts.t, err, "failed to get queue size")
	assert.Equal(ts.t, expectedSize, size, "queue size mismatch")
	return ts
}

func (ts *StakerTest) AssertLeaderGroupSize(expectedSize uint64) *StakerTest {
	size, err := ts.LeaderGroupSize()
	assert.NoError(ts.t, err, "failed to get leader group size")
	assert.Equal(ts.t, expectedSize, size, "leader group size mismatch")
	return ts
}

func (ts *StakerTest) AssertNext(prev thor.Address, expected thor.Address) *StakerTest {
	next, err := ts.Staker.Next(prev)
	assert.NoError(ts.t, err, "failed to get next validator after %s", prev.String())
	assert.Equal(ts.t, expected, next, "next validator mismatch after %s", prev.String())
	return ts
}

func (ts *StakerTest) LockedStake() (uint64, uint64) {
	vet, weight, err := ts.Staker.LockedStake()
	assert.NoError(ts.t, err, "failed to get locked stake")
	return vet, weight
}

func (ts *StakerTest) QueuedStake() uint64 {
	vet, err := ts.Staker.QueuedStake()
	assert.NoError(ts.t, err, "failed to get queued stake")
	return vet
}

func (ts *StakerTest) FirstActive() (thor.Address, *validation.Validation) {
	first, err := ts.Staker.FirstActive()
	assert.NoError(ts.t, err)
	val, err := ts.Staker.GetValidation(first)
	assert.NoError(ts.t, err)
	return first, val
}

func (ts *StakerTest) FirstQueued() (thor.Address, *validation.Validation) {
	first, err := ts.Staker.FirstQueued()
	assert.NoError(ts.t, err)
	val, err := ts.Staker.GetValidation(first)
	assert.NoError(ts.t, err)
	return first, val
}

func (ts *StakerTest) Next(prev thor.Address) (thor.Address, *validation.Validation) {
	first, err := ts.Staker.Next(prev)
	assert.NoError(ts.t, err)
	val, err := ts.Staker.GetValidation(first)
	assert.NoError(ts.t, err)
	return first, val
}

func (ts *StakerTest) GetValidation(addr thor.Address) *validation.Validation {
	val, err := ts.Staker.GetValidation(addr)
	assert.NoError(ts.t, err, "failed to get validator %s", addr.String())
	return val
}

func (ts *StakerTest) GetValidationErrors(addr thor.Address, errMsg string) *StakerTest {
	_, err := ts.Staker.GetValidation(addr)
	assert.NotNil(ts.t, err, "expected error when getting validator %s", addr.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when getting validator %s", addr.String())
	return ts
}

func (ts *StakerTest) GetAggregation(addr thor.Address) *aggregation.Aggregation {
	agg, err := ts.aggregationService.GetAggregation(addr)
	assert.NoError(ts.t, err, "failed to get aggregation for validator %s", addr.String())
	return agg
}

func (ts *StakerTest) GetDelegation(delegationID *big.Int) *delegation.Delegation {
	del, _, err := ts.Staker.GetDelegation(delegationID)
	assert.NoError(ts.t, err, "failed to get delegation %s", delegationID.String())
	return del
}

func (ts *StakerTest) AddValidationErrors(
	validator, endorser thor.Address,
	period uint32,
	stake uint64,
	errMsg string,
) *StakerTest {
	ts.addContractVET(stake)
	err := ts.Staker.AddValidation(validator, endorser, period, stake)
	assert.NotNil(ts.t, err, "expected error when adding validator %s with endorser %s", validator.String(), endorser.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when adding validator %s with endorser %s", validator.String(), endorser.String())
	ts.subContractVET(stake)
	return ts
}

func (ts *StakerTest) AddValidation(
	validator, endorser thor.Address,
	period uint32,
	stake uint64,
) *StakerTest {
	ts.addContractVET(stake)
	err := ts.Staker.AddValidation(validator, endorser, period, stake)
	assert.NoError(ts.t, err, "failed to add validator %s with endorser %s", validator.String(), endorser.String())
	return ts
}

func (ts *StakerTest) UpdateContractBalance(amount uint64) *StakerTest {
	addr := ts.Staker.Address()
	current, err := ts.state.GetBalance(addr)
	assert.NoError(ts.t, err, "failed to get contract balance")
	if current == nil {
		current = big.NewInt(0)
	}
	newBalance := new(big.Int).Add(current, big.NewInt(int64(amount)))
	assert.NoError(ts.t, ts.state.SetBalance(addr, newBalance))
	return ts
}

func (ts *StakerTest) SignalExit(validator, endorser thor.Address, currentBlock uint32) *StakerTest {
	err := ts.Staker.SignalExit(validator, endorser, currentBlock)
	assert.NoError(ts.t, err, "failed to signal exit for validator %s with endorser %s", validator.String(), endorser.String())
	return ts
}

func (ts *StakerTest) SignalExitErrors(validator, endorser thor.Address, currentBlock uint32, errMsg string) *StakerTest {
	err := ts.Staker.SignalExit(validator, endorser, currentBlock)
	assert.NotNil(ts.t, err, "expected error when signaling exit for validator %s with endorser %s", validator.String(), endorser.String())
	assert.ErrorContains(
		ts.t,
		err,
		errMsg,
		"expected error message when signaling exit for validator %s with endorser %s",
		validator.String(),
		endorser.String(),
	)
	return ts
}

func (ts *StakerTest) IncreaseStake(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
) *StakerTest {
	ts.addContractVET(amount)
	err := ts.Staker.IncreaseStake(addr, endorser, amount)
	assert.NoError(ts.t, err, "failed to increase stake for validator %s by %d: %v", addr.String(), amount, err)
	return ts
}

func (ts *StakerTest) IncreaseStakeErrors(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
	errMsg string,
) *StakerTest {
	ts.addContractVET(amount)
	err := ts.Staker.IncreaseStake(addr, endorser, amount)
	assert.NotNil(ts.t, err, "expected error when increasing stake for validator %s by %d", addr.String(), amount)
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when increasing stake for validator %s by %d", addr.String(), amount)
	ts.subContractVET(amount)
	return ts
}

func (ts *StakerTest) DecreaseStake(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
) *StakerTest {
	err := ts.Staker.DecreaseStake(addr, endorser, amount)
	assert.NoError(ts.t, err, "failed to decrease stake for validator %s by %d: %v", addr.String(), amount, err)
	return ts
}

func (ts *StakerTest) DecreaseStakeErrors(
	addr thor.Address,
	endorser thor.Address,
	amount uint64,
	errMsg string,
) *StakerTest {
	err := ts.Staker.DecreaseStake(addr, endorser, amount)
	assert.NotNil(ts.t, err, "expected error when decreasing stake for validator %s by %d", addr.String(), amount)
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when decreasing stake for validator %s by 1", addr.String())
	return ts
}

func (ts *StakerTest) WithdrawStake(validator, endorser thor.Address, block uint32, expectedOut uint64) *StakerTest {
	amount, err := ts.Staker.WithdrawStake(validator, endorser, block)
	assert.NoError(ts.t, err, "failed to withdraw stake for validator %s with endorser %s at block %d: %v", validator.String(), endorser.String(), block, err)
	assert.Equal(
		ts.t,
		amount, expectedOut,
		"withdrawn amount mismatch for validator %s with endorser %s at block %d",
		validator.String(),
		endorser.String(),
		block,
	)
	ts.subContractVET(amount)
	return ts
}

func (ts *StakerTest) WithdrawStakeErrors(validator, endorser thor.Address, block uint32, errMsg string) *StakerTest {
	_, err := ts.Staker.WithdrawStake(validator, endorser, block)
	assert.ErrorContains(
		ts.t,
		err,
		errMsg,
		"expected error message when withdrawing stake for validator %s with endorser %s at block %d",
		validator.String(),
		endorser.String(),
		block,
	)
	return ts
}

func (ts *StakerTest) SetBeneficiary(
	validator thor.Address,
	endorser thor.Address,
	beneficiary thor.Address,
) *StakerTest {
	err := ts.Staker.SetBeneficiary(validator, endorser, beneficiary)
	assert.NoError(ts.t, err, "failed to set beneficiary for validator %s with endorser %s: %v", validator.String(), endorser.String(), err)
	return ts
}

func (ts *StakerTest) SetBeneficiaryErrors(
	validator thor.Address,
	endorser thor.Address,
	beneficiary thor.Address,
	errMsg string,
) *StakerTest {
	err := ts.Staker.SetBeneficiary(validator, endorser, beneficiary)
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

func (ts *StakerTest) AssertWithdrawable(
	validator thor.Address,
	block uint32,
	expectedWithdrawable uint64,
) *StakerTest {
	withdrawable, err := ts.GetWithdrawable(validator, block)
	assert.NoError(ts.t, err, "failed to get withdrawable amount for validator %s at block %d: %v", validator.String(), block, err)
	assert.Equal(ts.t, expectedWithdrawable, withdrawable, "withdrawable amount mismatch for validator %s", validator.String())
	return ts
}

func (ts *StakerTest) SetOnline(id thor.Address, blockNum uint32, online bool) *StakerTest {
	err := ts.Staker.SetOnline(id, blockNum, online)
	assert.NoError(ts.t, err, "failed to set online status for validator %s: %v", id.String(), err)
	return ts
}

func (ts *StakerTest) AddDelegation(
	validator thor.Address,
	amount uint64,
	multiplier uint8,
	currentBlock uint32,
) *big.Int {
	ts.addContractVET(amount)
	delegationID, err := ts.Staker.AddDelegation(validator, amount, multiplier, currentBlock)
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

func (ts *StakerTest) AddDelegationErrors(
	validator thor.Address,
	amount uint64,
	multiplier uint8,
	currentBlock uint32,
	errMsg string,
) *StakerTest {
	ts.addContractVET(amount)
	_, err := ts.Staker.AddDelegation(validator, amount, multiplier, currentBlock)
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
	ts.subContractVET(amount)
	return ts
}

func (ts *StakerTest) AssertHasDelegations(node thor.Address, expected bool) *StakerTest {
	hasDelegations, err := ts.HasDelegations(node)
	assert.NoError(ts.t, err, "failed to check delegations for validator %s: %v", node.String(), err)
	assert.Equal(ts.t, expected, hasDelegations, "delegation presence mismatch for validator %s", node.String())
	return ts
}

func (ts *StakerTest) SignalDelegationExit(delegationID *big.Int, currentBlock uint32) *StakerTest {
	assert.NoError(ts.t, ts.Staker.SignalDelegationExit(delegationID, currentBlock))
	return ts
}

func (ts *StakerTest) SignalDelegationExitErrors(delegationID *big.Int, currentBlock uint32, errMsg string) *StakerTest {
	err := ts.Staker.SignalDelegationExit(delegationID, currentBlock)
	assert.NotNil(ts.t, err, "expected error when signaling exit for delegation %s", delegationID.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when signaling exit for delegation %s", delegationID.String())
	return ts
}

func (ts *StakerTest) WithdrawDelegation(delegationID *big.Int, expectedOut uint64, currentBlock uint32) *StakerTest {
	amount, err := ts.Staker.WithdrawDelegation(delegationID, currentBlock)
	assert.NoError(ts.t, err, "failed to withdraw delegation %s: %v", delegationID.String(), err)
	assert.Equal(ts.t, expectedOut, amount, "withdrawn amount mismatch for delegation %s", delegationID.String())

	ts.subContractVET(amount)

	return ts
}

func (ts *StakerTest) WithdrawDelegationErrors(delegationID *big.Int, currentBlock uint32, errMsg string) *StakerTest {
	_, err := ts.Staker.WithdrawDelegation(delegationID, currentBlock)
	assert.NotNil(ts.t, err, "expected error when withdrawing for delegation %s", delegationID.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when withdrawing for delegation %s", delegationID.String())
	return ts
}

func (ts *StakerTest) AssertDelegatorRewards(
	validationID thor.Address,
	period uint32,
	expectedReward *big.Int,
) *StakerTest {
	reward, err := ts.GetDelegatorRewards(validationID, period)
	assert.NoError(ts.t, err, "failed to get rewards for validator %s at period %d: %v", validationID.String(), period, err)
	assert.Equal(ts.t, expectedReward, reward, "reward mismatch for validator %s at period %d", validationID.String(), period)
	return ts
}

func (ts *StakerTest) AssertCompletedPeriods(
	validationID thor.Address,
	expectedPeriods uint32,
	currentBlock uint32,
) *StakerTest {
	val, err := ts.Staker.GetValidation(validationID)
	assert.NotNil(ts.t, val, "validation %s not found", validationID.String())
	assert.NoError(ts.t, err, "failed to get validation %s", validationID.String())
	periods, err := val.CompletedIterations(currentBlock)
	assert.NoError(ts.t, err, "failed to get completed periods for validator %s: %v", validationID.String(), err)
	assert.Equal(ts.t, expectedPeriods, periods, "completed periods mismatch for validator %s", validationID.String())
	return ts
}

func (ts *StakerTest) AssertTotals(validationID thor.Address, expected *validation.Totals) *StakerTest {
	totals, err := ts.GetValidationTotals(validationID)
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

func (ts *StakerTest) AssertGlobalWithdrawable(expected uint64) *StakerTest {
	withdrawable, err := ts.globalStatsService.GetWithdrawableStake()
	assert.NoError(ts.t, err, "failed to get global withdrawable")

	assert.Equal(ts.t, expected, withdrawable, "total withdrawable mismatch")

	return ts
}

func (ts *StakerTest) AssertGlobalCooldown(expected uint64) *StakerTest {
	cooldown, err := ts.globalStatsService.GetCooldownStake()
	assert.NoError(ts.t, err, "failed to get global cooldown")

	assert.Equal(ts.t, expected, cooldown, "total cooldown mismatch")

	return ts
}

func (ts *StakerTest) ActivateNext(block uint32) *StakerTest {
	mbp, err := ts.Staker.params.Get(thor.KeyMaxBlockProposers)
	assert.NoError(ts.t, err, "failed to get max block proposers")
	_, err = ts.activateNextValidation(block, mbp.Uint64())
	assert.NoError(ts.t, err, "failed to activate next validator at block %d", block)
	return ts
}

func (ts *StakerTest) ActivateNextErrors(block uint32, errMsg string) *StakerTest {
	mbp, err := ts.Staker.params.Get(thor.KeyMaxBlockProposers)
	assert.NoError(ts.t, err, "failed to get max block proposers")
	_, err = ts.activateNextValidation(block, mbp.Uint64())
	assert.NotNil(ts.t, err, "expected error when activating next validator at block %d", block)
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when activating next validator at block %d", block)
	return ts
}

func (ts *StakerTest) Housekeep(block uint32) *StakerTest {
	_, err := ts.Staker.Housekeep(block)
	assert.NoError(ts.t, err, "failed to perform housekeeping at block %d", block)
	return ts
}

func (ts *StakerTest) HousekeepWithUpdates(block uint32) (*StakerTest, bool) {
	updates, err := ts.Staker.Housekeep(block)
	assert.NoError(ts.t, err, "failed to perform housekeeping at block %d", block)
	return ts, updates
}

func (ts *StakerTest) Transition(block uint32) *StakerTest {
	_, err := ts.transition(block)
	assert.NoError(ts.t, err, "failed to transition at block %d", block)
	return ts
}

func (ts *StakerTest) IncreaseDelegatorsReward(node thor.Address, reward *big.Int, currentBlock uint32) *StakerTest {
	assert.NoError(ts.t, ts.Staker.IncreaseDelegatorsReward(node, reward, currentBlock))
	return ts
}

// ExitValidator manually exits a validator, skipping the housekeeping part
func (ts *StakerTest) ExitValidator(node thor.Address) *StakerTest {
	exit, err := ts.validationService.ExitValidator(node)
	assert.NoError(ts.t, err, "failed to exit validator %s", node.String())
	aggExit, err := ts.aggregationService.Exit(node)
	assert.NoError(ts.t, err, "failed to exit aggregation for validator %s", node.String())
	assert.NoError(ts.t, ts.globalStatsService.ApplyExit(exit, aggExit))
	return ts
}

func (ts *StakerTest) ExitValidatorErrors(node thor.Address, errMsg string) *StakerTest {
	_, err := ts.validationService.ExitValidator(node)
	assert.NotNil(ts.t, err, "expected error when exiting validator %s", node.String())
	assert.ErrorContains(ts.t, err, errMsg, "expected error message when exiting validator %s", node.String())
	return ts
}

func (ts *StakerTest) AssertValidation(addr thor.Address) *ValidationAssertions {
	return assertValidation(ts.t, ts, addr)
}

func (ts *StakerTest) AssertAggregation(validationID thor.Address) *AggregationAssertions {
	return assertAggregation(ts.t, ts.Staker, validationID)
}

func (ts *StakerTest) AssertDelegation(delegationID *big.Int) *DelegationAssertions {
	return assertDelegation(ts.t, ts.Staker, delegationID)
}

type ValidationAssertions struct {
	addr      thor.Address
	validator *validation.Validation
	t         *testing.T
}

func assertValidation(t *testing.T, test *StakerTest, addr thor.Address) *ValidationAssertions {
	validator, err := test.Staker.GetValidation(addr)
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

func (va *ValidationAssertions) CompletedIterations(expected, block uint32) *ValidationAssertions {
	completed, err := va.validator.CompletedIterations(block)
	assert.NoError(va.t, err)
	assert.Equal(va.t, expected, completed)
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

func (da *DelegationAssertions) IsLocked(expected bool, currentBlock uint32) *DelegationAssertions {
	locked, err := da.delegation.IsLocked(da.validation, currentBlock)
	assert.NoError(da.t, err)
	assert.Equal(da.t, expected, locked, "delegation %s locked state mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) IsStarted(expected bool, currentBlock uint32) *DelegationAssertions {
	started, err := da.delegation.Started(da.validation, currentBlock)
	assert.NoError(da.t, err)
	assert.Equal(da.t, expected, started, "delegation %s started state mismatch", da.delegationID.String())
	return da
}

func (da *DelegationAssertions) IsFinished(expected bool, currentBlock uint32) *DelegationAssertions {
	ended, err := da.delegation.Ended(da.validation, currentBlock)
	assert.NoError(da.t, err)
	assert.Equal(da.t, expected, ended, "delegation %s finished state mismatch", da.delegationID.String())
	return da
}
