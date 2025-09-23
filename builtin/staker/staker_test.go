// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/validation"

	"github.com/vechain/thor/v2/thor"
)

func TestValidation_SignalExit_InvalidEndorser(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))
	wrong := thor.BytesToAddress([]byte("wrong"))

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.SignalExitErrors(id, wrong, 10, "endorser required")
}

func TestValidation_SignalExit_NotActive(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.SignalExitErrors(id, end, 10, "can't signal exit while not active")
}

func TestService_IncreaseStake_UnknownValidator(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)
	id := thor.BytesToAddress([]byte("unknown"))
	staker.IncreaseStakeErrors(id, id, 1, "validation does not exist")
}

func TestValidation_IncreaseStake_InvalidEndorser(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))
	wrong := thor.BytesToAddress([]byte("wrong"))

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.IncreaseStakeErrors(id, wrong, 10, "endorser required")
}

func TestValidation_IncreaseStake_StatusExit(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET+1)
	staker.WithdrawStake(id, end, 1, MinStakeVET+1)
	staker.IncreaseStakeErrors(id, end, 5, "validator exited")
}

func TestValidation_IncreaseStake_ActiveHasExitBlock(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.AssertValidation(id).Status(validation.StatusQueued)
	staker.ActivateNext(0)
	staker.SignalExit(id, end, 10)

	staker.IncreaseStakeErrors(id, end, 5, "validator has signaled exit, cannot increase stake")
}

func TestValidation_DecreaseStake_UnknownValidator(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("unknown"))
	staker.DecreaseStakeErrors(id, id, 1, "validation does not exist")
}

func TestValidation_DecreaseStake_InvalidEndorser(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)
	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))
	wrong := thor.BytesToAddress([]byte("wrong"))

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.DecreaseStakeErrors(id, wrong, 1, "endorser required")
}

func TestValidation_DecreaseStake_StatusExit(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.AssertValidation(id).Status(validation.StatusQueued)
	staker.ActivateNext(0)
	staker.SignalExit(id, end, 10)

	staker.DecreaseStakeErrors(id, end, 5, "validator has signaled exit, cannot decrease stake")
}

func TestValidation_DecreaseStake_ActiveHasExitBlock(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.AssertValidation(id).Status(validation.StatusQueued)
	staker.ActivateNext(0)

	staker.SignalExit(id, end, 10)
	staker.DecreaseStakeErrors(id, end, 5, "validator has signaled exit, cannot decrease stake")
}

func TestValidation_DecreaseStake_ActiveTooLowNextPeriod(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.DecreaseStakeErrors(id, end, 100, "next period stake is lower than minimum stake")
}

func TestValidation_DecreaseStake_ActiveSuccess(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET+100).ActivateNext(0)
	staker.DecreaseStake(id, end, 100)
	staker.AssertValidation(id).Status(validation.StatusActive).
		PendingUnlockVET(100).
		LockedVET(MinStakeVET + 100).
		WithdrawableVET(0)
}

func TestValidation_DecreaseStake_QueuedTooLowNextPeriod(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET)
	staker.DecreaseStakeErrors(id, end, 100, "next period stake is lower than minimum stake")
}

func TestValidation_DecreaseStake_QueuedSuccess(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	end := id

	staker.AddValidation(id, end, thor.MediumStakingPeriod(), MinStakeVET+100)
	staker.DecreaseStake(id, end, 100)
	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), withdrawable)

	staker.AssertValidation(id).QueuedVET(MinStakeVET).WithdrawableVET(100)
}

func TestValidation_WithdrawStake_InvalidEndorser(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	wrong := thor.BytesToAddress([]byte("wrong"))
	endorsor := id
	staker.AddValidation(id, endorsor, thor.LowStakingPeriod(), MinStakeVET)
	staker.WithdrawStakeErrors(id, wrong, 0, "endorser required")
}

func TestValidationAdd_Error(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id1 := thor.BytesToAddress([]byte("id1"))

	staker.AddValidationErrors(id1, id1, uint32(1), MinStakeVET, "period is out of boundaries")
	staker.AddValidationErrors(id1, id1, thor.LowStakingPeriod(), 0, "stake is below minimum")
	staker.AddValidation(id1, id1, thor.LowStakingPeriod(), MinStakeVET)
	staker.AddValidationErrors(id1, id1, thor.LowStakingPeriod(), MinStakeVET, "validator already exists")
}

func TestValidation_SetBeneficiary_Error(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	id := thor.BytesToAddress([]byte("v"))
	wrong := thor.BytesToAddress([]byte("wrong"))
	endorsor := id
	staker.AddValidation(id, endorsor, thor.LowStakingPeriod(), MinStakeVET)

	staker.SetBeneficiaryErrors(id, wrong, id, "endorser required")
	staker.WithdrawStake(id, id, 0, MinStakeVET)
	staker.SetBeneficiaryErrors(id, id, id, "validator has exited or signaled exit, cannot set beneficiary")
}

func TestDelegation_Add_InputValidation(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	staker.AddDelegationErrors(thor.Address{}, 1, 0, 10, "multiplier cannot be 0")
}

func TestDelegation_SignalExit(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	v := thor.BytesToAddress([]byte("v"))
	staker.AddValidation(v, v, thor.MediumStakingPeriod(), MinStakeVET)

	id := staker.AddDelegation(v, 3, 100, 10)

	val, err := staker.validationService.GetValidation(v)
	assert.NoError(t, err)
	assert.False(t, val == nil)

	staker.ActivateNext(0)

	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	staker.SignalDelegationExit(id, 10)
	withdrawable, err = staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	del2 := staker.GetDelegation(id)
	assert.NotNil(t, del2.LastIteration)
	assert.Equal(t, uint32(1), *del2.LastIteration)

	staker.SignalDelegationExitErrors(id, 10, "delegation is already signaled exit")
}

func TestDelegation_SignalExit_AlreadyWithdrawn(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	v := thor.BytesToAddress([]byte("v"))
	staker.AddValidation(v, v, thor.MediumStakingPeriod(), MinStakeVET)

	id := staker.AddDelegation(v, 3, 100, 10)

	withdrawable, err := staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	withdrawable, err = staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)
	staker.WithdrawDelegation(id, 3, 10)

	withdrawable, err = staker.globalStatsService.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	staker.SignalDelegationExitErrors(id, 10, "delegation has already been withdrawn")
}

func TestDelegation_SignalExit_Empty(t *testing.T) {
	staker, _ := newStakerV2(t, 0, 101, false)

	staker.SignalDelegationExitErrors(big.NewInt(2), 10, "delegation is empty")
}
