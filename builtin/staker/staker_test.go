// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func newTestStaker() *testStaker {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	addr := thor.BytesToAddress([]byte("staker"))
	return &testStaker{addr, st, New(addr, st, params.New(addr, st), nil)}
}

func TestValidation_SignalExit_InvalidEndorser(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))
	wrong := thor.BytesToAddress([]byte("wrong"))

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.SignalExit(id, wrong, 10)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidation_SignalExit_NotActive(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.SignalExit(id, end, 10)
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
	wrong := thor.BytesToAddress([]byte("wrong"))

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.IncreaseStake(id, wrong, 10)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidation_IncreaseStake_StatusExit(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET+1))

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
	assert.False(t, val == nil)

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.SignalExit(id, end, 10)
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
	wrong := thor.BytesToAddress([]byte("wrong"))

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	err := staker.DecreaseStake(id, wrong, 1)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidation_DecreaseStake_StatusExit(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), 100))

	val, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val == nil)

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.SignalExit(id, end, 10)
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
	assert.False(t, val == nil)

	staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	err = staker.SignalExit(id, end, 10)
	assert.NoError(t, err)

	err = staker.DecreaseStake(id, end, 5)
	assert.ErrorContains(t, err, "validator has signaled exit, cannot decrease stake")
}

func TestValidation_DecreaseStake_ActiveTooLowNextPeriod(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET))
	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	assert.NoError(t, err)

	err = staker.DecreaseStake(id, end, 100)
	assert.ErrorContains(t, err, "next period stake is lower than minimum stake")
}

func TestValidation_DecreaseStake_ActiveSuccess(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.validationService.Add(id, end, thor.MediumStakingPeriod(), MinStakeVET+100))

	val, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val == nil)

	_, err = staker.validationService.ActivateValidator(id, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})
	assert.NoError(t, err)

	err = staker.DecreaseStake(id, end, 100)
	assert.NoError(t, err)

	v, err := staker.validationService.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), v.PendingUnlockVET)
	assert.Equal(t, MinStakeVET+100, v.LockedVET)
	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)
}

func TestValidation_DecreaseStake_QueuedTooLowNextPeriod(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET))

	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	assert.NoError(t, err)

	err = staker.DecreaseStake(id, end, 100)
	assert.ErrorContains(t, err, "next period stake is lower than minimum stake")
}

func TestValidation_DecreaseStake_QueuedSuccess(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET+100))

	_, err := staker.Housekeep(thor.MediumStakingPeriod())
	assert.NoError(t, err)

	assert.NoError(t, staker.DecreaseStake(id, end, 100))
	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	v, err := staker.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), v.QueuedVET)
	assert.Equal(t, MinStakeVET+100, v.LockedVET)
	assert.Equal(t, uint64(0), v.WithdrawableVET)
}

func TestValidation_WithdrawStake_InvalidEndorser(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	wrong := thor.BytesToAddress([]byte("wrong"))
	endorsor := id
	assert.NoError(t, staker.AddValidation(id, endorsor, thor.LowStakingPeriod(), MinStakeVET))

	amt, err := staker.WithdrawStake(id, wrong, 0)
	assert.Equal(t, uint64(0), amt)
	assert.ErrorContains(t, err, "endorser required")
}

func TestValidationAdd_Error(t *testing.T) {
	staker := newTestStaker()

	id1 := thor.BytesToAddress([]byte("id1"))

	assert.ErrorContains(t, staker.AddValidation(id1, id1, uint32(1), MinStakeVET), "period is out of boundaries")
	assert.ErrorContains(t, staker.AddValidation(id1, id1, thor.LowStakingPeriod(), 0), "stake is below minimum")
	assert.NoError(t, staker.AddValidation(id1, id1, thor.LowStakingPeriod(), MinStakeVET))
	assert.ErrorContains(t, staker.AddValidation(id1, id1, thor.LowStakingPeriod(), MinStakeVET), "validator already exists")
}

func TestValidation_SetBeneficiary_Error(t *testing.T) {
	staker := newTestStaker()

	id := thor.BytesToAddress([]byte("v"))
	wrong := thor.BytesToAddress([]byte("wrong"))
	endorsor := id
	assert.NoError(t, staker.AddValidation(id, endorsor, thor.LowStakingPeriod(), MinStakeVET))

	assert.ErrorContains(t, staker.SetBeneficiary(id, wrong, id), "endorser required")

	_, err := staker.WithdrawStake(id, id, 0)
	assert.NoError(t, err)

	assert.ErrorContains(t, staker.SetBeneficiary(id, id, id), "validator has exited or signaled exit, cannot set beneficiary")
}

func TestDelegation_Add_InputValidation(t *testing.T) {
	staker := newTestStaker()

	_, err := staker.AddDelegation(thor.Address{}, 1, 0, 10)
	assert.ErrorContains(t, err, "multiplier cannot be 0")
}

func TestDelegation_SignalExit(t *testing.T) {
	staker := newTestStaker()

	v := thor.BytesToAddress([]byte("v"))
	assert.NoError(t, staker.AddValidation(v, v, thor.MediumStakingPeriod(), MinStakeVET))

	id, err := staker.AddDelegation(v, 3, 100, 10)
	assert.NoError(t, err)

	val, err := staker.validationService.GetValidation(v)
	assert.NoError(t, err)
	assert.False(t, val == nil)

	staker.validationService.ActivateValidator(v, val, 0, &globalstats.Renewal{
		LockedIncrease: stakes.NewWeightedStake(0, 0),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 0,
	})

	_, _, err = staker.GetDelegation(id)
	assert.NoError(t, err)
	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	assert.NoError(t, staker.SignalDelegationExit(id, 10))
	withdrawable, err = staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	del2, _, err := staker.GetDelegation(id)
	assert.NoError(t, err)
	assert.NotNil(t, del2.LastIteration)
	assert.Equal(t, uint32(1), *del2.LastIteration)

	assert.ErrorContains(t, staker.SignalDelegationExit(id, 10), "delegation is already signaled exit")
}

func TestDelegation_SignalExit_AlreadyWithdrawn(t *testing.T) {
	staker := newTestStaker()

	v := thor.BytesToAddress([]byte("v"))
	assert.NoError(t, staker.AddValidation(v, v, thor.MediumStakingPeriod(), MinStakeVET))

	id, err := staker.AddDelegation(v, 3, 100, 10)
	assert.NoError(t, err)

	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	_, _, err = staker.GetDelegation(id)
	assert.NoError(t, err)

	withdrawable, err = staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)
	amt, err := staker.WithdrawDelegation(id, 10)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), amt)

	withdrawable, err = staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	assert.ErrorContains(t, staker.SignalDelegationExit(id, 10), "delegation has already been withdrawn")
}

func TestDelegation_SignalExit_Empty(t *testing.T) {
	staker := newTestStaker()

	assert.ErrorContains(t, staker.SignalDelegationExit(big.NewInt(2), 10), "delegation is empty")
}

func Test_AddDelegation_WhileValidatorExiting(t *testing.T) {
	staker, _ := newStaker(t, 3, 3, true)

	first, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(first)
	assert.NoError(t, err)

	_, err = staker.AddDelegation(first, 10_000, 150, 10)
	assert.NoError(t, err)

	assert.NoError(t, staker.SignalExit(first, val.Endorser, 20))

	_, err = staker.AddDelegation(first, 10_000, 150, 10)
	assert.ErrorContains(t, err, "cannot add delegation to exiting validator")

	_, err = staker.Housekeep(val.Period)
	assert.NoError(t, err)
}

// Add a check to avoid delegations to be added to exitting validators
// Add the Queued Aggregations AND Validations Stake in the housekeep
func Test_Increase_WhileValidatorExiting(t *testing.T) {
	staker, _ := newStaker(t, 3, 3, true)

	first, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(first)
	assert.NoError(t, err)

	// add before validator signals exit, should be okay
	err = staker.IncreaseStake(first, val.Endorser, 10_000)
	assert.NoError(t, err)

	assert.NoError(t, staker.SignalExit(first, val.Endorser, 20))

	// add after validator signals exit, should fail
	err = staker.IncreaseStake(first, val.Endorser, 10_000)
	assert.Error(t, err)

	// housekeep should clean up the queued delegation
	_, err = staker.Housekeep(val.Period)
	assert.NoError(t, err)

	// housekeep should clean up the queued delegation
	_, err = staker.Housekeep(val.Period + val.Period)
	assert.NoError(t, err)
}

func Test_WithdrawDelegation_before_SignalExit(t *testing.T) {
	staker, _ := newStaker(t, 3, 3, true)

	first, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(first)
	assert.NoError(t, err)

	delID, err := staker.AddDelegation(first, 10_000, 150, 10)
	assert.NoError(t, err)

	delStake, err := staker.WithdrawDelegation(delID, 15)
	assert.NoError(t, err)
	assert.Equal(t, delStake, uint64(10_000))

	assert.NoError(t, staker.SignalExit(first, val.Endorser, 20))

	_, err = staker.AddDelegation(first, 10_000, 150, 10)
	assert.ErrorContains(t, err, "cannot add delegation to exiting validator")

	_, err = staker.Housekeep(val.Period)
	assert.NoError(t, err)
}

func Test_WithdrawDelegation_after_SignalExit(t *testing.T) {
	staker, _ := newStaker(t, 3, 3, true)

	first, err := staker.FirstActive()
	assert.NoError(t, err)
	val, err := staker.GetValidation(first)
	assert.NoError(t, err)

	delID, err := staker.AddDelegation(first, 10_000, 150, 10)
	assert.NoError(t, err)

	assert.NoError(t, staker.SignalExit(first, val.Endorser, 20))

	delStake, err := staker.WithdrawDelegation(delID, 15)
	assert.NoError(t, err)
	assert.Equal(t, delStake, uint64(10_000))

	_, err = staker.AddDelegation(first, 10_000, 150, 10)
	assert.ErrorContains(t, err, "cannot add delegation to exiting validator")

	_, err = staker.Housekeep(val.Period)
	assert.NoError(t, err)
}
