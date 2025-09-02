// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package validation

import (
	"encoding/binary"
	"math/big"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func poisonExitSlot(st *state.State, contract thor.Address, block uint32) {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)
	slot := thor.Blake2b(key[:], slotExitEpochs.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
}

func newSvc() (*Service, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("valsvc"))
	svc := New(
		solidity.NewContext(addr, st, nil),
		/* min */ 1,
		/* max */ 1_000_000,
	)
	return svc, addr, st
}

func poisonValidationSlot(st *state.State, contract thor.Address, id thor.Address) {
	slot := thor.Blake2b(id.Bytes(), slotValidations.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
}

func poisonQueueSlot(st *state.State, contract thor.Address) {
	st.SetRawStorage(contract, slotQueuedGroupSize, rlp.RawValue{0xFF})
}

func TestService_SetGetValidation_RoundTrip(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v1"))
	val := &Validation{
		Endorser:           thor.BytesToAddress([]byte("e1")),
		Period:             2,
		CompleteIterations: 0,
		Status:             StatusQueued,
	}

	assert.NoError(t, svc.repo.addValidation(id, val))

	got, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, val.Endorser, got.Endorser)
	assert.Equal(t, uint32(2), got.Period)
	assert.Equal(t, StatusQueued, got.Status)
	assert.Nil(t, got.OfflineBlock)
}

func TestService_GetValidation_Error(t *testing.T) {
	svc, addr, st := newSvc()
	id := thor.BytesToAddress([]byte("v2"))

	poisonValidationSlot(st, addr, id)

	_, err := svc.GetValidation(id)
	assert.ErrorContains(t, err, "failed to get validator")
}

func TestService_LeaderGroup_Iterator_Empty(t *testing.T) {
	svc, _, _ := newSvc()

	err := svc.LeaderGroupIterator(func(_ thor.Address, _ *Validation) error { return nil })
	assert.NoError(t, err)

	group, err := svc.LeaderGroup()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(group))
}

func TestService_ActivateAndExit_Flow(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v3"))
	val := &Validation{
		Endorser:           id,
		Period:             2,
		Status:             StatusQueued,
		QueuedVET:          uint64(100),
		LockedVET:          uint64(0),
		PendingUnlockVET:   uint64(0),
		WithdrawableVET:    uint64(0),
		Weight:             uint64(0),
		CompleteIterations: 0,
	}
	assert.NoError(t, svc.repo.addValidation(id, val))

	renew, err := (&Validation{QueuedVET: uint64(100), LockedVET: uint64(0), Weight: uint64(0)}).renew(uint64(0))
	assert.NoError(t, err)

	val, err = svc.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	_, err = svc.ActivateValidator(id, val, 1, renew)
	assert.NoError(t, err)

	after, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, after.Status)
	assert.Equal(t, uint64(100), after.LockedVET)
	assert.Equal(t, uint64(0), after.QueuedVET)

	_, err = svc.ExitValidator(id)
	assert.NoError(t, err)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, v2.Status)
	assert.Equal(t, uint64(0), v2.LockedVET)
}

func TestService_LeaderGroup_ReturnsActiveOnly(t *testing.T) {
	svc, _, _ := newSvc()

	q := thor.BytesToAddress([]byte("q"))
	assert.NoError(t, svc.repo.addValidation(q, &Validation{Status: StatusQueued}))
	a := thor.BytesToAddress([]byte("a"))
	assert.NoError(t, svc.repo.addValidation(a, &Validation{Status: StatusActive}))

	val, err := svc.GetValidation(a)
	assert.NoError(t, err)
	_, err = svc.ActivateValidator(a, val, 1, &globalstats.Renewal{LockedIncrease: &stakes.WeightedStake{}, LockedDecrease: &stakes.WeightedStake{}})
	assert.NoError(t, err)

	group, err := svc.LeaderGroup()
	assert.NoError(t, err)

	leaders := make(map[thor.Address]bool)
	for _, leader := range group {
		leaders[leader.Address] = true
	}

	_, inQueued := leaders[q]
	_, inActive := leaders[a]
	assert.False(t, inQueued)
	assert.True(t, inActive)
}

func TestService_QueuedAndLeader_LenAndHead(t *testing.T) {
	svc, _, _ := newSvc()

	q1 := thor.BytesToAddress([]byte("q1"))
	q2 := thor.BytesToAddress([]byte("q2"))
	assert.NoError(t, svc.Add(q1, q1, thor.LowStakingPeriod(), 1))
	assert.NoError(t, svc.Add(q2, q2, thor.LowStakingPeriod(), 1))

	id, val, err := svc.NextToActivate(10)
	assert.NoError(t, err)
	assert.Equal(t, q1, id)

	_, err = svc.ActivateValidator(id, val, 1, &globalstats.Renewal{LockedIncrease: &stakes.WeightedStake{}, LockedDecrease: &stakes.WeightedStake{}})
	assert.NoError(t, err)

	headActive, err := svc.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, q1, *headActive)

	headQueued, err := svc.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, q2, *headQueued)
}

func TestService_IsActive_Flag(t *testing.T) {
	svc, _, _ := newSvc()
	ok, err := svc.IsActive()
	assert.NoError(t, err)
	assert.False(t, ok)

	id := thor.BytesToAddress([]byte("x"))
	assert.NoError(t, svc.Add(id, id, thor.LowStakingPeriod(), 1))

	val, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	_, err = svc.ActivateValidator(
		id,
		val,
		thor.LowStakingPeriod(),
		&globalstats.Renewal{LockedIncrease: &stakes.WeightedStake{}, LockedDecrease: &stakes.WeightedStake{}},
	)
	assert.NoError(t, err)

	ok, err = svc.IsActive()
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestService_SignalExit_SetsExitBlockAndPersists(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: end, Status: StatusActive,
		StartBlock: 100, Period: 10, CompleteIterations: 2,
	}))

	val, err := svc.GetExistingValidation(id)
	assert.NoError(t, err)

	err = svc.SignalExit(id, val)
	assert.NoError(t, err)

	after, err := svc.GetValidation(id)
	assert.NoError(t, err)
	if assert.NotNil(t, after.ExitBlock) {
		assert.Equal(t, uint32(130), *after.ExitBlock)
	}
}

func TestService_SignalExit_ExitBlockLimitReached(t *testing.T) {
	svc, _, _ := newSvc()

	validator := thor.BytesToAddress([]byte("validator"))
	endorser := validator
	validation := &Validation{
		Endorser:           endorser,
		Status:             StatusActive,
		StartBlock:         100,
		Period:             10,
		CompleteIterations: 0,
	}

	assert.NoError(t, svc.repo.addValidation(validator, validation))

	minBlock := validation.StartBlock + validation.Period*(validation.CurrentIteration())

	for idx := range 20 {
		blockNum := minBlock + uint32(idx*int(thor.EpochLength()))

		exitValidator := thor.BytesToAddress([]byte("exit" + strconv.Itoa(idx)))

		assert.NoError(t, svc.repo.setExit(blockNum, exitValidator))
	}

	val, err := svc.GetExistingValidation(validator)
	assert.NoError(t, err)

	err = svc.SignalExit(validator, val)

	assert.Error(t, err)
	assert.ErrorContains(t, err, "max try reached")

	updatedVal, err := svc.GetValidation(validator)
	assert.NoError(t, err)
	assert.Nil(t, updatedVal.ExitBlock)
}

func TestService_SignalExit_SetExitBlock_Error(t *testing.T) {
	svc, contract, st := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: end, Status: StatusActive,
		StartBlock: 10, Period: 5, CompleteIterations: 0,
	}))

	poisonExitSlot(st, contract, 15)

	val, err := svc.GetExistingValidation(id)
	assert.NoError(t, err)
	err = svc.SignalExit(id, val)
	assert.Error(t, err)
}

func TestService_IncreaseStake_SuccessQueued(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("q"))
	endorser := id
	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: endorser, Status: StatusQueued, QueuedVET: uint64(7),
	}))

	val, err := svc.GetExistingValidation(id)
	assert.NoError(t, err)

	assert.NoError(t, svc.IncreaseStake(id, val, uint64(3)))

	v, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(10), v.QueuedVET)
}

func TestService_IncreaseStake_SuccessActiveNoExit(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("a"))
	endorser := id
	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive, QueuedVET: uint64(0),
	}))

	val, err := svc.GetExistingValidation(id)
	assert.NoError(t, err)

	assert.NoError(t, svc.IncreaseStake(id, val, uint64(4)))

	v, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(4), v.QueuedVET)
}

func TestService_WithdrawStake_QueuedToExit(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("q"))
	endorser := id
	assert.NoError(t, svc.Add(id, endorser, thor.LowStakingPeriod(), uint64(50)))

	val, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, val.Status)

	val, err = svc.GetExistingValidation(id)
	assert.NoError(t, err)

	amt, _, err := svc.WithdrawStake(id, val, 0)
	assert.NoError(t, err)
	assert.Equal(t, uint64(50), amt)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, v2.Status)
	assert.Equal(t, uint64(0), v2.QueuedVET)
	assert.Equal(t, uint64(0), v2.WithdrawableVET)
}

func TestService_WithdrawStake_ClearCooldownWhenMatured(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("ex"))
	endorser := id
	eb := uint32(10)
	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: endorser, Status: StatusExit,
		ExitBlock: &eb, CooldownVET: uint64(40), WithdrawableVET: uint64(5),
	}))

	thor.SetConfig(thor.Config{
		CooldownPeriod: 1,
	})

	val, err := svc.GetExistingValidation(id)
	assert.NoError(t, err)

	amt, _, err := svc.WithdrawStake(id, val, 11)
	assert.NoError(t, err)
	assert.Equal(t, uint64(45), amt)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), v2.CooldownVET)
	assert.Equal(t, uint64(0), v2.WithdrawableVET)
}

func TestService_GetDelegatorRewards_Positive(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: id, Status: StatusActive,
		CompleteIterations: 1,
	}))

	assert.NoError(t, svc.IncreaseDelegatorsReward(id, big.NewInt(100)))

	got, err := svc.GetDelegatorRewards(id, 2)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), got)

	assert.NoError(t, svc.IncreaseDelegatorsReward(id, big.NewInt(40)))
	got, err = svc.GetDelegatorRewards(id, 2)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(140), got)
}

func TestService_WithdrawStake_ActiveClearsQueuedAndWithdrawable(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("act"))
	endorser := id
	assert.NoError(t, svc.repo.addValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive,
		QueuedVET: uint64(12), WithdrawableVET: uint64(3),
	}))

	val, err := svc.GetExistingValidation(id)
	assert.NoError(t, err)

	amt, _, err := svc.WithdrawStake(id, val, 0)
	assert.NoError(t, err)
	assert.Equal(t, uint64(15), amt)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, v2.Status)
	assert.Equal(t, uint64(0), v2.QueuedVET)
	assert.Equal(t, uint64(0), v2.WithdrawableVET)
}

func TestService_GetDelegatorRewards_Error(t *testing.T) {
	svc, contract, st := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	var pb [4]byte
	binary.BigEndian.PutUint32(pb[:], 3)
	key := thor.Blake2b([]byte("rewards"), id.Bytes(), pb[:])

	slot := thor.Blake2b(key.Bytes(), slotRewards.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})

	_, err := svc.GetDelegatorRewards(id, 3)
	assert.Error(t, err)
}

func TestService_ValidatorQueueNext_Order(t *testing.T) {
	svc, _, _ := newSvc()

	q1 := thor.BytesToAddress([]byte("q1"))
	q2 := thor.BytesToAddress([]byte("q2"))
	q3 := thor.BytesToAddress([]byte("q3"))
	assert.NoError(t, svc.Add(q1, q1, thor.LowStakingPeriod(), uint64(1)))
	assert.NoError(t, svc.Add(q2, q2, thor.LowStakingPeriod(), uint64(1)))
	assert.NoError(t, svc.Add(q3, q3, thor.LowStakingPeriod(), uint64(1)))

	head, err := svc.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, q1, *head)

	n2, err := svc.NextEntry(*head)
	assert.NoError(t, err)
	assert.Equal(t, q2, n2)

	n3, err := svc.NextEntry(n2)
	assert.NoError(t, err)
	assert.Equal(t, q3, n3)

	n4, err := svc.NextEntry(n3)
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, n4)
}

func TestService_LeaderGroupNext_Order(t *testing.T) {
	svc, _, _ := newSvc()

	a1 := thor.BytesToAddress([]byte("a1"))
	a2 := thor.BytesToAddress([]byte("a2"))
	a3 := thor.BytesToAddress([]byte("a3"))
	for _, id := range []thor.Address{a1, a2, a3} {
		assert.NoError(t, svc.Add(id, id, thor.LowStakingPeriod(), uint64(1)))
		idPtr, _, err := svc.NextToActivate(10)
		assert.NoError(t, err)
		assert.Equal(t, id, idPtr)
		val, err := svc.GetValidation(idPtr)
		assert.NoError(t, err)
		assert.False(t, val.IsEmpty())
		_, err = svc.ActivateValidator(idPtr, val, 1, &globalstats.Renewal{LockedIncrease: &stakes.WeightedStake{}, LockedDecrease: &stakes.WeightedStake{}})
		assert.NoError(t, err)
	}

	head, err := svc.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, a1, *head)

	n2, err := svc.NextEntry(*head)
	assert.NoError(t, err)
	assert.Equal(t, a2, n2)

	n3, err := svc.NextEntry(n2)
	assert.NoError(t, err)
	assert.Equal(t, a3, n3)

	n4, err := svc.NextEntry(n3)
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, n4)
}

func TestService_GetCompletedPeriods(t *testing.T) {
	svc, _, _ := newSvc()

	a1 := thor.BytesToAddress([]byte("a1"))
	a2 := thor.BytesToAddress([]byte("a2"))
	a3 := thor.BytesToAddress([]byte("a3"))
	for _, id := range []thor.Address{a1, a2, a3} {
		assert.NoError(t, svc.Add(id, id, thor.LowStakingPeriod(), uint64(1)))
		idPtr, _, err := svc.NextToActivate(10)
		assert.NoError(t, err)
		assert.Equal(t, id, idPtr)
		val, err := svc.GetValidation(idPtr)
		assert.NoError(t, err)
		assert.False(t, val.IsEmpty())
		_, err = svc.ActivateValidator(idPtr, val, 1, &globalstats.Renewal{LockedIncrease: &stakes.WeightedStake{}, LockedDecrease: &stakes.WeightedStake{}})
		assert.NoError(t, err)
	}

	periods, err := svc.GetCompletedPeriods(a1)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), periods)
}

func TestService_GetQueuedAndLeaderGroups(t *testing.T) {
	svc, _, _ := newSvc()

	a1 := thor.BytesToAddress([]byte("a1"))
	a2 := thor.BytesToAddress([]byte("a2"))
	a3 := thor.BytesToAddress([]byte("a3"))
	for _, id := range []thor.Address{a1, a2, a3} {
		assert.NoError(t, svc.Add(id, id, thor.LowStakingPeriod(), uint64(1)))
	}

	queuedCnt, err := svc.QueuedGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), queuedCnt)

	leaderCnt, err := svc.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), leaderCnt)

	idPtr, _, err := svc.NextToActivate(10)
	assert.NoError(t, err)
	assert.Equal(t, a1, idPtr)
	val, err := svc.GetValidation(idPtr)
	assert.False(t, val.IsEmpty())
	_, err = svc.ActivateValidator(idPtr, val, 1, &globalstats.Renewal{LockedIncrease: &stakes.WeightedStake{}, LockedDecrease: &stakes.WeightedStake{}})
	assert.NoError(t, err)

	queuedCnt, err = svc.QueuedGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), queuedCnt)

	leaderCnt, err = svc.LeaderGroupSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), leaderCnt)

	val, err = svc.GetLeaderGroupHead()
	assert.NoError(t, err)
	assert.Equal(t, a1, val.Endorser)
	assert.Nil(t, val.Beneficiary)
	assert.Equal(t, uint64(1), val.LockedVET)
	assert.Equal(t, uint64(1), val.Weight)
	assert.Equal(t, thor.LowStakingPeriod(), val.Period)
	assert.Equal(t, uint32(0), val.CompleteIterations)
	assert.Equal(t, StatusActive, val.Status)
	assert.Equal(t, uint32(1), val.StartBlock)
	assert.Nil(t, val.ExitBlock)
	assert.Nil(t, val.OfflineBlock)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(0), val.QueuedVET)
	assert.Equal(t, uint64(0), val.CooldownVET)
	assert.Equal(t, uint64(0), val.WithdrawableVET)
}

func TestService_Add_Error(t *testing.T) {
	svc, addr, st := newSvc()
	id1 := thor.BytesToAddress([]byte("id1"))
	id2 := thor.BytesToAddress([]byte("id2"))

	assert.NoError(t, svc.Add(id1, id1, thor.LowStakingPeriod(), uint64(1)))
	poisonValidationSlot(st, addr, id1)
	_, err := svc.GetExistingValidation(id1)
	assert.Error(t, err)

	slot := thor.Blake2b(id1.Bytes(), slotValidations.Bytes())
	st.SetRawStorage(addr, slot, rlp.RawValue{0x0})
	poisonQueueSlot(st, addr)
	assert.Error(t, svc.Add(id2, id2, thor.LowStakingPeriod(), uint64(1)))
}

func TestService_Evict(t *testing.T) {
	svc, addr, st := newSvc()
	id1 := thor.BytesToAddress([]byte("id1"))

	assert.NoError(t, svc.Add(id1, id1, thor.LowStakingPeriod(), uint64(1)))

	assert.NoError(t, svc.Evict(id1, 5))
	val, err := svc.GetValidation(id1)
	assert.NoError(t, err)
	expectedExitBlock := uint32(5) + thor.EpochLength()
	assert.Equal(t, &expectedExitBlock, val.ExitBlock)

	poisonExitSlot(st, addr, 7+thor.EpochLength())
	assert.Error(t, svc.Evict(id1, 7))

	poisonValidationSlot(st, addr, id1)
	assert.Error(t, svc.Evict(id1, 8))
}

func TestService_SetBeneficiary(t *testing.T) {
	svc, _, _ := newSvc()
	id1 := thor.BytesToAddress([]byte("id1"))
	assert.NoError(t, svc.Add(id1, id1, thor.LowStakingPeriod(), uint64(1)))

	val, err := svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Nil(t, val.Beneficiary)

	assert.NoError(t, svc.SetBeneficiary(id1, val, id1))
	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Equal(t, &id1, val.Beneficiary)

	assert.NoError(t, svc.SetBeneficiary(id1, val, thor.Address{}))
	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Nil(t, val.Beneficiary)
}

func TestService_UpdateOfflineBlock(t *testing.T) {
	svc, _, _ := newSvc()

	id1 := thor.BytesToAddress([]byte("id1"))
	assert.NoError(t, svc.Add(id1, id1, thor.LowStakingPeriod(), uint64(1)))

	val, err := svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Nil(t, val.OfflineBlock)

	assert.NoError(t, svc.UpdateOfflineBlock(id1, 2, false))

	expectedOfflineBlk := uint32(2)
	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Equal(t, &expectedOfflineBlk, val.OfflineBlock)

	assert.NoError(t, svc.UpdateOfflineBlock(id1, 2, true))

	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Nil(t, val.OfflineBlock)
}

func TestService_Renew(t *testing.T) {
	svc, _, _ := newSvc()

	id1 := thor.BytesToAddress([]byte("id1"))
	assert.NoError(t, svc.Add(id1, id1, thor.LowStakingPeriod(), uint64(50)))

	val, err := svc.GetValidation(id1)
	assert.NoError(t, err)

	err = svc.IncreaseStake(id1, val, uint64(600))
	assert.NoError(t, err)
	err = svc.DecreaseStake(id1, val, uint64(300))
	assert.NoError(t, err)

	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), val.LockedVET)
	assert.Equal(t, uint64(0), val.Weight)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(350), val.QueuedVET)
	assert.Equal(t, uint64(0), val.CooldownVET)
	assert.Equal(t, uint64(300), val.WithdrawableVET)

	delta, err := svc.Renew(id1, uint64(1500))

	assert.NoError(t, err)
	assert.Equal(t, uint64(350), delta.LockedIncrease.VET)
	assert.Equal(t, uint64(700), delta.LockedIncrease.Weight)
	assert.Equal(t, uint64(350), delta.QueuedDecrease)

	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(350), val.LockedVET)
	assert.Equal(t, uint64(2200), val.Weight)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(0), val.QueuedVET)
	assert.Equal(t, uint64(0), val.CooldownVET)
	assert.Equal(t, uint64(300), val.WithdrawableVET)

	err = svc.IncreaseStake(id1, val, uint64(400))
	assert.NoError(t, err)
	err = svc.DecreaseStake(id1, val, uint64(200))
	assert.NoError(t, err)

	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(350), val.LockedVET)
	assert.Equal(t, uint64(2200), val.Weight)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(200), val.QueuedVET)
	assert.Equal(t, uint64(0), val.CooldownVET)
	assert.Equal(t, uint64(500), val.WithdrawableVET)

	delta, err = svc.Renew(id1, uint64(1500))
	assert.NoError(t, err)
	assert.Equal(t, uint64(200), delta.LockedIncrease.VET)
	assert.Equal(t, uint64(400), delta.LockedIncrease.Weight)
	assert.Equal(t, uint64(200), delta.QueuedDecrease)

	val, err = svc.GetValidation(id1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(550), val.LockedVET)
	assert.Equal(t, uint64(550*2+1500), val.Weight)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(0), val.QueuedVET)
	assert.Equal(t, uint64(0), val.CooldownVET)
	assert.Equal(t, uint64(500), val.WithdrawableVET)
}
