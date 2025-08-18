// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package validation

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func poisonExitSlot(st *state.State, contract thor.Address, block uint32) {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))
	slot := thor.Blake2b(bigBlock.Bytes(), slotExitEpochs.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
}

func newSvc() (*Service, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("valsvc"))
	svc := New(
		solidity.NewContext(addr, st, nil),
		/* cooldown */ 1,
		/* epochLen */ 1,
		/* low */ 1 /* med */, 2 /* high */, 3,
		/* min */ big.NewInt(1),
		/* max */ big.NewInt(1_000_000),
	)
	return svc, addr, st
}

func poisonValidationSlot(st *state.State, contract thor.Address, id thor.Address) {
	slot := thor.Blake2b(id.Bytes(), slotValidations.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
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

	assert.NoError(t, svc.SetValidation(id, val, true))

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
		QueuedVET:          big.NewInt(100),
		LockedVET:          big.NewInt(0),
		PendingUnlockVET:   big.NewInt(0),
		WithdrawableVET:    big.NewInt(0),
		Weight:             big.NewInt(0),
		CompleteIterations: 0,
	}
	assert.NoError(t, svc.SetValidation(id, val, true))

	renew := (&Validation{QueuedVET: big.NewInt(100)}).Renew()

	_, err := svc.ActivateValidator(id, 1, renew)
	assert.NoError(t, err)

	after, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, after.Status)
	assert.Equal(t, big.NewInt(100), after.LockedVET)
	assert.Equal(t, big.NewInt(0), after.QueuedVET)

	exit, err := svc.ExitValidator(id)
	assert.NoError(t, err)
	assert.True(t, exit.ExitedTVL.Sign() >= 0)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, v2.Status)
	assert.Equal(t, big.NewInt(0), v2.LockedVET)
}

func TestService_LeaderGroup_ReturnsActiveOnly(t *testing.T) {
	svc, _, _ := newSvc()

	q := thor.BytesToAddress([]byte("q"))
	assert.NoError(t, svc.SetValidation(q, &Validation{Status: StatusQueued}, true))
	a := thor.BytesToAddress([]byte("a"))
	assert.NoError(t, svc.SetValidation(a, &Validation{Status: StatusActive}, true))

	_, err := svc.ActivateValidator(a, 1, &delta.Renewal{NewLockedWeight: big.NewInt(0)})
	assert.NoError(t, err)

	group, err := svc.LeaderGroup()
	assert.NoError(t, err)
	_, inQueued := group[q]
	_, inActive := group[a]
	assert.False(t, inQueued)
	assert.True(t, inActive)
}

func TestService_QueuedAndLeader_LenAndHead(t *testing.T) {
	svc, _, _ := newSvc()

	q1 := thor.BytesToAddress([]byte("q1"))
	q2 := thor.BytesToAddress([]byte("q2"))
	assert.NoError(t, svc.Add(q1, q1, 1, big.NewInt(1)))
	assert.NoError(t, svc.Add(q2, q2, 1, big.NewInt(1)))

	id, err := svc.NextToActivate(big.NewInt(10))
	assert.NoError(t, err)
	assert.Equal(t, q1, *id)

	_, err = svc.ActivateValidator(*id, 1, &delta.Renewal{NewLockedWeight: big.NewInt(0)})
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
	assert.NoError(t, svc.Add(id, id, 1, big.NewInt(1)))
	_, err = svc.ActivateValidator(id, 1, &delta.Renewal{NewLockedWeight: big.NewInt(0)})
	assert.NoError(t, err)

	ok, err = svc.IsActive()
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestService_SignalExit_InvalidEndorser(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	end := thor.BytesToAddress([]byte("endorse"))

	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: end, Status: StatusActive, Period: 2, StartBlock: 100, CompleteIterations: 0,
	}, true))

	err := svc.SignalExit(id, thor.BytesToAddress([]byte("wrong")))
	assert.ErrorContains(t, err, "invalid endorser for node")
}

func TestService_SignalExit_NotActive(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: end, Status: StatusQueued, Period: 2, StartBlock: 0,
	}, true))

	err := svc.SignalExit(id, end)
	assert.ErrorContains(t, err, "can't signal exit while not active")
}

func TestService_SignalExit_SetsExitBlockAndPersists(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: end, Status: StatusActive,
		StartBlock: 100, Period: 10, CompleteIterations: 2,
	}, true))

	err := svc.SignalExit(id, end)
	assert.NoError(t, err)

	after, err := svc.GetValidation(id)
	assert.NoError(t, err)
	if assert.NotNil(t, after.ExitBlock) {
		assert.Equal(t, uint32(130), *after.ExitBlock)
	}
}

func TestService_SignalExit_SetExitBlock_Error(t *testing.T) {
	svc, contract, st := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	end := id

	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: end, Status: StatusActive,
		StartBlock: 10, Period: 5, CompleteIterations: 0,
	}, true))

	poisonExitSlot(st, contract, 15)

	err := svc.SignalExit(id, end)
	assert.Error(t, err)
}

func TestService_IncreaseStake_UnknownValidator(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("unknown"))
	err := svc.IncreaseStake(id, id, big.NewInt(1))
	assert.ErrorContains(t, err, "failed to get validator")
}

func TestService_IncreaseStake_InvalidEndorser(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := thor.BytesToAddress([]byte("endorse"))
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusQueued, QueuedVET: big.NewInt(0),
	}, true))

	err := svc.IncreaseStake(id, thor.BytesToAddress([]byte("wrong")), big.NewInt(10))
	assert.ErrorContains(t, err, "invalid endorser")
}

func TestService_IncreaseStake_StatusExit(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusExit,
	}, true))

	err := svc.IncreaseStake(id, endorser, big.NewInt(5))
	assert.ErrorContains(t, err, "validator status is not queued or active")
}

func TestService_IncreaseStake_ActiveHasExitBlock(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	eb := uint32(1)
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive, ExitBlock: &eb,
	}, true))

	err := svc.IncreaseStake(id, endorser, big.NewInt(5))
	assert.ErrorContains(t, err, "has signaled exit, cannot increase stake")
}

func TestService_IncreaseStake_SuccessQueued(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("q"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusQueued, QueuedVET: big.NewInt(7),
	}, true))

	assert.NoError(t, svc.IncreaseStake(id, endorser, big.NewInt(3)))

	v, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(10), v.QueuedVET)
}

func TestService_IncreaseStake_SuccessActiveNoExit(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("a"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive, QueuedVET: big.NewInt(0),
	}, true))

	assert.NoError(t, svc.IncreaseStake(id, endorser, big.NewInt(4)))

	v, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(4), v.QueuedVET)
}

func TestService_DecreaseStake_UnknownValidator(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("unknown"))
	ok, err := svc.DecreaseStake(id, id, big.NewInt(1))
	assert.False(t, ok)
	assert.ErrorContains(t, err, "failed to get validator")
}

func TestService_DecreaseStake_InvalidEndorser(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := thor.BytesToAddress([]byte("endorse"))
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusQueued, QueuedVET: big.NewInt(10),
	}, true))

	ok, err := svc.DecreaseStake(id, thor.BytesToAddress([]byte("wrong")), big.NewInt(1))
	assert.False(t, ok)
	assert.ErrorContains(t, err, "invalid endorser")
}

func TestService_DecreaseStake_StatusExit(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusExit,
	}, true))

	ok, err := svc.DecreaseStake(id, endorser, big.NewInt(1))
	assert.False(t, ok)
	assert.ErrorContains(t, err, "validator status is not queued or active")
}

func TestService_DecreaseStake_ActiveHasExitBlock(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	eb := uint32(1)
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive, ExitBlock: &eb,
		LockedVET: big.NewInt(10),
	}, true))

	ok, err := svc.DecreaseStake(id, endorser, big.NewInt(1))
	assert.False(t, ok)
	assert.ErrorContains(t, err, "has signaled exit, cannot decrease stake")
}

func TestService_DecreaseStake_ActiveTooLowNextPeriod(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive,
		LockedVET: big.NewInt(1), PendingUnlockVET: big.NewInt(0),
	}, true))

	ok, err := svc.DecreaseStake(id, endorser, big.NewInt(1))
	assert.False(t, ok)
	assert.ErrorContains(t, err, "next period stake is too low for validator")
}

func TestService_DecreaseStake_ActiveSuccess(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive,
		LockedVET: big.NewInt(5), PendingUnlockVET: big.NewInt(0),
	}, true))

	ok, err := svc.DecreaseStake(id, endorser, big.NewInt(2))
	assert.False(t, ok)
	assert.NoError(t, err)

	v, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), v.PendingUnlockVET)
	assert.Equal(t, big.NewInt(5), v.LockedVET)
}

func TestService_DecreaseStake_QueuedTooLowNextPeriod(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("q"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusQueued,
		QueuedVET: big.NewInt(1), WithdrawableVET: big.NewInt(0),
	}, true))

	ok, err := svc.DecreaseStake(id, endorser, big.NewInt(1))
	assert.False(t, ok)
	assert.ErrorContains(t, err, "next period stake is too low for validator")
}

func TestService_DecreaseStake_QueuedSuccess(t *testing.T) {
	svc, _, _ := newSvc()
	id := thor.BytesToAddress([]byte("q"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusQueued,
		QueuedVET: big.NewInt(5), WithdrawableVET: big.NewInt(0),
	}, true))

	ok, err := svc.DecreaseStake(id, endorser, big.NewInt(2))
	assert.True(t, ok)
	assert.NoError(t, err)

	v, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(3), v.QueuedVET)
	assert.Equal(t, big.NewInt(2), v.WithdrawableVET)
}

func TestService_WithdrawStake_InvalidEndorser(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	endorser := id
	assert.NoError(t, svc.Add(id, endorser, 1, big.NewInt(10)))

	val, err := svc.GetValidation(id)
	assert.NoError(t, err)

	amt, err := svc.WithdrawStake(val, id, thor.BytesToAddress([]byte("wrong")), 0)
	assert.Equal(t, big.NewInt(0).String(), amt.String())
	assert.ErrorContains(t, err, "invalid endorser")
}

func TestService_WithdrawStake_QueuedToExit(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("q"))
	endorser := id
	assert.NoError(t, svc.Add(id, endorser, 1, big.NewInt(50)))

	val, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusQueued, val.Status)

	amt, err := svc.WithdrawStake(val, id, endorser, 0)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(50), amt)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusExit, v2.Status)
	assert.Equal(t, big.NewInt(0), v2.QueuedVET)
	assert.Equal(t, big.NewInt(0), v2.WithdrawableVET)
}

func TestService_WithdrawStake_ClearCooldownWhenMatured(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("ex"))
	endorser := id
	eb := uint32(10)
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusExit,
		ExitBlock: &eb, CooldownVET: big.NewInt(40), WithdrawableVET: big.NewInt(5),
	}, true))

	val, err := svc.GetValidation(id)
	assert.NoError(t, err)

	amt, err := svc.WithdrawStake(val, id, endorser, 11)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(45), amt)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), v2.CooldownVET)
	assert.Equal(t, big.NewInt(0), v2.WithdrawableVET)
}

func TestService_WithdrawStake_ActiveClearsQueuedAndWithdrawable(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("act"))
	endorser := id
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: endorser, Status: StatusActive,
		QueuedVET: big.NewInt(12), WithdrawableVET: big.NewInt(3),
	}, true))

	val, err := svc.GetValidation(id)
	assert.NoError(t, err)

	amt, err := svc.WithdrawStake(val, id, endorser, 0)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(15), amt)

	v2, err := svc.GetValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, StatusActive, v2.Status)
	assert.Equal(t, big.NewInt(0), v2.QueuedVET)
	assert.Equal(t, big.NewInt(0), v2.WithdrawableVET)
}

func TestService_GetDelegatorRewards_Positive(t *testing.T) {
	svc, _, _ := newSvc()

	id := thor.BytesToAddress([]byte("v"))
	assert.NoError(t, svc.SetValidation(id, &Validation{
		Endorser: id, Status: StatusActive,
		CompleteIterations: 1,
	}, true))

	assert.NoError(t, svc.IncreaseDelegatorsReward(id, big.NewInt(100)))

	got, err := svc.GetDelegatorRewards(id, 2)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), got)

	assert.NoError(t, svc.IncreaseDelegatorsReward(id, big.NewInt(40)))
	got, err = svc.GetDelegatorRewards(id, 2)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(140), got)
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
	assert.NoError(t, svc.Add(q1, q1, 1, big.NewInt(1)))
	assert.NoError(t, svc.Add(q2, q2, 1, big.NewInt(1)))
	assert.NoError(t, svc.Add(q3, q3, 1, big.NewInt(1)))

	head, err := svc.FirstQueued()
	assert.NoError(t, err)
	assert.Equal(t, q1, *head)

	n2, err := svc.ValidatorQueueNext(*head)
	assert.NoError(t, err)
	assert.Equal(t, q2, n2)

	n3, err := svc.ValidatorQueueNext(n2)
	assert.NoError(t, err)
	assert.Equal(t, q3, n3)

	n4, err := svc.ValidatorQueueNext(n3)
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, n4)
}

func TestService_LeaderGroupNext_Order(t *testing.T) {
	svc, _, _ := newSvc()

	a1 := thor.BytesToAddress([]byte("a1"))
	a2 := thor.BytesToAddress([]byte("a2"))
	a3 := thor.BytesToAddress([]byte("a3"))
	for _, id := range []thor.Address{a1, a2, a3} {
		assert.NoError(t, svc.Add(id, id, 1, big.NewInt(1)))
		idPtr, err := svc.NextToActivate(big.NewInt(10))
		assert.NoError(t, err)
		assert.Equal(t, id, *idPtr)
		_, err = svc.ActivateValidator(*idPtr, 1, &delta.Renewal{NewLockedWeight: big.NewInt(0)})
		assert.NoError(t, err)
	}

	head, err := svc.FirstActive()
	assert.NoError(t, err)
	assert.Equal(t, a1, *head)

	n2, err := svc.LeaderGroupNext(*head)
	assert.NoError(t, err)
	assert.Equal(t, a2, n2)

	n3, err := svc.LeaderGroupNext(n2)
	assert.NoError(t, err)
	assert.Equal(t, a3, n3)

	n4, err := svc.LeaderGroupNext(n3)
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, n4)
}
