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
	"github.com/vechain/thor/v2/builtin/staker/delta"
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

	qVET, err = svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
}

func TestService_AddRemove_Queued(t *testing.T) {
	svc, _, _ := newSvc()

	st := stakes.NewWeightedStakeWithMultiplier(1000, 200) // weight: 2000
	assert.NoError(t, svc.AddQueued(st))

	assert.NoError(t, svc.RemoveQueued(st))
	qVET, qW, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
	assert.Equal(t, uint64(0), qW)
}

func TestService_ApplyRenewal(t *testing.T) {
	svc, _, _ := newSvc()

	// seed some queued for decrease
	assert.NoError(t, svc.AddQueued(stakes.NewWeightedStakeWithMultiplier(500, 200))) // weight 1000

	r := &delta.Renewal{
		LockedIncrease: stakes.NewWeightedStake(500, 1000),
		LockedDecrease: stakes.NewWeightedStake(200, 400),
		QueuedDecrease: stakes.NewWeightedStake(500, 1000),
	}
	assert.NoError(t, svc.ApplyRenewal(r))

	lockedV, lockedW, err := svc.GetLockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(300), lockedV)
	assert.Equal(t, uint64(600), lockedW)

	qVET, qW, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
	assert.Equal(t, uint64(0), qW)
}

func TestService_ApplyExit(t *testing.T) {
	svc, _, _ := newSvc()

	assert.NoError(t, svc.AddQueued(stakes.NewWeightedStakeWithMultiplier(1000, 200)))
	assert.NoError(t, svc.ApplyRenewal(&delta.Renewal{
		LockedIncrease: stakes.NewWeightedStake(1000, 2000),
		LockedDecrease: stakes.NewWeightedStake(0, 0),
		QueuedDecrease: stakes.NewWeightedStake(1000, 2000),
	}))

	assert.NoError(t, svc.AddQueued(stakes.NewWeightedStakeWithMultiplier(300, 200))) // weight 600

	exit := &delta.Exit{
		ExitedTVL:      stakes.NewWeightedStake(400, 800),
		QueuedDecrease: stakes.NewWeightedStake(300, 600),
	}
	assert.NoError(t, svc.ApplyExit(exit))

	lockedV, lockedW, err := svc.GetLockedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(600), lockedV)  // 1000 - 400
	assert.Equal(t, uint64(1200), lockedW) // 2000 - 800

	qVET, qW, err := svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), qVET)
	assert.Equal(t, uint64(0), qW)
}

func TestService_QueuedStake_GetQueuedError(t *testing.T) {
	svc, addr, st := newSvc()
	poisonUintSlot(st, addr, slotQueued)

	_, _, err := svc.GetQueuedStake()
	assert.Error(t, err)
}

func TestService_GetLockedVET_GetLockedVetError(t *testing.T) {
	svc, addr, st := newSvc()
	poisonUintSlot(st, addr, slotLocked)

	_, _, err := svc.GetLockedStake()
	assert.Error(t, err)
}
