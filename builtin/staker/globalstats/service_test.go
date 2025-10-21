// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package globalstats

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func poisonUintSlot(st *state.State, contract thor.Address, slot thor.Bytes32) {
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
}

func newSvc() (*Service, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("gs"))
	svc := New(solidity.NewContext(addr, st, nil))
	return svc, addr, st
}

func TestService_QueuedStake_Empty(t *testing.T) {
	svc, _, _ := newSvc()

	qVET, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
}

func TestService_AddRemove_Queued(t *testing.T) {
	svc, _, _ := newSvc()

	stake := uint64(1000)
	assert.NoError(t, svc.AddQueued(stake))

	assert.NoError(t, svc.RemoveQueued(stake))
	qVET, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
}

func TestService_ApplyRenewal(t *testing.T) {
	svc, _, _ := newSvc()

	// seed some queued for decrease
	assert.NoError(t, svc.AddQueued(500)) // weight 1000

	r := &Renewal{
		LockedIncrease: stakes.NewWeightedStake(500, 1000),
		LockedDecrease: stakes.NewWeightedStake(200, 400),
		QueuedDecrease: 500,
	}
	assert.NoError(t, svc.ApplyRenewal(r))

	lockedV, lockedW, err := svc.GetLockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(300), lockedV)
	assert.Equal(t, uint64(600), lockedW)

	qVET, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
}

func TestService_ApplyExit(t *testing.T) {
	svc, _, _ := newSvc()

	assert.NoError(t, svc.AddQueued(1000))
	assert.NoError(t, svc.ApplyRenewal(&Renewal{
		LockedIncrease: stakes.NewWeightedStake(1000, 2000),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: 1000,
	}))

	assert.NoError(t, svc.AddQueued(300)) // weight 600

	valExit := &Exit{
		ExitedTVL:      stakes.NewWeightedStake(200, 400),
		QueuedDecrease: 0,
	}

	aggExit := &Exit{
		ExitedTVL:      stakes.NewWeightedStake(200, 400),
		QueuedDecrease: 300,
	}

	assert.NoError(t, svc.ApplyExit(valExit, aggExit))

	lockedV, lockedW, err := svc.GetLockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(600), lockedV)  // 1000 - 400
	assert.Equal(t, uint64(1200), lockedW) // 2000 - 800

	qVET, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
}

func TestService_QueuedStake_GetQueuedError(t *testing.T) {
	svc, addr, st := newSvc()
	poisonUintSlot(st, addr, slotQueued)

	_, err := svc.GetQueuedStake()
	assert.Error(t, err)
}

func TestService_GetLockedVET_GetLockedVetError(t *testing.T) {
	svc, addr, st := newSvc()
	poisonUintSlot(st, addr, slotLocked)

	_, _, err := svc.GetLockedStake()
	assert.Error(t, err)
}

func TestService_GetWithdrawableStake(t *testing.T) {
	svc, _, _ := newSvc()

	withdrawable, err := svc.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), withdrawable)

	err = svc.AddWithdrawable(10)
	assert.NoError(t, err)

	withdrawable, err = svc.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(10), withdrawable)

	err = svc.RemoveWithdrawable(9)
	assert.NoError(t, err)

	withdrawable, err = svc.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), withdrawable)

	err = svc.RemoveWithdrawable(9)
	assert.ErrorContains(t, err, "withdrawable underflow occurred")

	withdrawable, err = svc.GetWithdrawableStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), withdrawable)
}

func TestService_GetCooldownStake(t *testing.T) {
	svc, _, _ := newSvc()

	cooldown, err := svc.GetCooldownStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), cooldown)

	err = svc.AddCooldown(15)
	assert.NoError(t, err)

	cooldown, err = svc.GetCooldownStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(15), cooldown)

	err = svc.RemoveCooldown(9)
	assert.NoError(t, err)

	cooldown, err = svc.GetCooldownStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), cooldown)

	err = svc.RemoveCooldown(9)
	assert.ErrorContains(t, err, "cooldown underflow occurred")

	cooldown, err = svc.GetCooldownStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(6), cooldown)
}
