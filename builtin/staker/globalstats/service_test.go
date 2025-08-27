// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package globalstats

import (
	"math/big"
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

func setUintSlot(st *state.State, contract thor.Address, slot thor.Bytes32, v *big.Int) {
	st.SetStorage(contract, slot, thor.BytesToBytes32(v.Bytes()))
}

var maxUint256 = func() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
}()

func newSvc() (*Service, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("gs"))
	svc := New(solidity.NewContext(addr, st, nil))
	return svc, addr, st
}

func TestService_QueuedStake_Empty(t *testing.T) {
	svc, _, _ := newSvc()
	qVET, qW, err := svc.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), qVET.String())
	assert.Equal(t, big.NewInt(0).String(), qW.String())

	qVET, err = svc.GetQueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), qVET.String())
}

func TestService_AddRemove_Queued(t *testing.T) {
	svc, _, _ := newSvc()

	st := stakes.NewWeightedStake(big.NewInt(1000), 200) // weight: 2000
	assert.NoError(t, svc.AddQueued(st))

	qVET, qW, err := svc.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), qVET)
	assert.Equal(t, big.NewInt(2000), qW)

	assert.NoError(t, svc.RemoveQueued(st))
	qVET, qW, err = svc.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), qVET.String())
	assert.Equal(t, big.NewInt(0).String(), qW.String())
}

func TestService_ApplyRenewal(t *testing.T) {
	svc, _, _ := newSvc()

	// seed some queued for decrease
	assert.NoError(t, svc.AddQueued(stakes.NewWeightedStake(big.NewInt(500), 200))) // weight 1000

	r := &delta.Renewal{
		NewLockedVET:         big.NewInt(300),
		NewLockedWeight:      big.NewInt(600),
		QueuedDecrease:       big.NewInt(500),
		QueuedDecreaseWeight: big.NewInt(1000),
	}
	assert.NoError(t, svc.ApplyRenewal(r))

	lockedV, lockedW, err := svc.GetLockedVET()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(300), lockedV)
	assert.Equal(t, big.NewInt(600), lockedW)

	qVET, qW, err := svc.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), qVET.String())
	assert.Equal(t, big.NewInt(0).String(), qW.String())
}

func TestService_ApplyExit(t *testing.T) {
	svc, _, _ := newSvc()

	assert.NoError(t, svc.ApplyRenewal(&delta.Renewal{
		NewLockedVET:    big.NewInt(1000),
		NewLockedWeight: big.NewInt(2000),
	}))

	assert.NoError(t, svc.AddQueued(stakes.NewWeightedStake(big.NewInt(300), 200))) // weight 600

	exit := &delta.Exit{
		ExitedTVL:            big.NewInt(400),
		ExitedTVLWeight:      big.NewInt(800),
		QueuedDecrease:       big.NewInt(300),
		QueuedDecreaseWeight: big.NewInt(600),
	}
	assert.NoError(t, svc.ApplyExit(exit))

	lockedV, lockedW, err := svc.GetLockedVET()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(600), lockedV)  // 1000 - 400
	assert.Equal(t, big.NewInt(1200), lockedW) // 2000 - 800

	qVET, qW, err := svc.QueuedStake()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).String(), qVET.String())
	assert.Equal(t, big.NewInt(0).String(), qW.String())
}

func TestService_QueuedStake_GetQueuedVetError(t *testing.T) {
	svc, addr, st := newSvc()
	poisonUintSlot(st, addr, slotQueuedVET)

	_, _, err := svc.QueuedStake()
	assert.Error(t, err)
}

func TestService_QueuedStake_GetQueuedWeightError(t *testing.T) {
	svc, addr, st := newSvc()
	setUintSlot(st, addr, slotQueuedVET, big.NewInt(0))
	poisonUintSlot(st, addr, slotQueuedWeight)

	_, _, err := svc.QueuedStake()
	assert.Error(t, err)
}

func TestService_GetLockedVET_GetLockedVetError(t *testing.T) {
	svc, addr, st := newSvc()
	poisonUintSlot(st, addr, slotLockedVET)

	_, _, err := svc.GetLockedVET()
	assert.Error(t, err)
}

func TestService_GetLockedVET_GetLockedWeightError(t *testing.T) {
	svc, addr, st := newSvc()
	setUintSlot(st, addr, slotLockedVET, big.NewInt(0))
	poisonUintSlot(st, addr, slotLockedWeight)

	_, _, err := svc.GetLockedVET()
	assert.Error(t, err)
}

func TestService_AddQueued_QueuedVetOverflow(t *testing.T) {
	svc, addr, st := newSvc()
	setUintSlot(st, addr, slotQueuedVET, new(big.Int).Set(maxUint256))
	setUintSlot(st, addr, slotQueuedWeight, new(big.Int).Set(maxUint256))

	err := svc.AddQueued(stakes.NewWeightedStake(big.NewInt(1), 1))
	assert.ErrorContains(t, err, "uint256 overflow")

	setUintSlot(st, addr, slotQueuedVET, new(big.Int).SetUint64(1))
	err = svc.AddQueued(stakes.NewWeightedStake(big.NewInt(1), 100))
	assert.ErrorContains(t, err, "uint256 overflow")
}

func TestService_RemoveQueued_QueuedVetNegative(t *testing.T) {
	svc, _, _ := newSvc()
	err := svc.RemoveQueued(stakes.NewWeightedStake(big.NewInt(1), 1))
	assert.ErrorContains(t, err, "queued-stake uint256 cannot be negative")
}

func TestService_ApplyRenewal_LockedVetOverflow(t *testing.T) {
	svc, addr, st := newSvc()
	setUintSlot(st, addr, slotLockedVET, new(big.Int).Set(maxUint256))
	r := &delta.Renewal{NewLockedVET: big.NewInt(1)}

	err := svc.ApplyRenewal(r)
	assert.ErrorContains(t, err, "uint256 overflow")
}

func TestService_ApplyRenewal_LockedWeightOverflow(t *testing.T) {
	svc, addr, st := newSvc()
	setUintSlot(st, addr, slotLockedWeight, new(big.Int).Set(maxUint256))
	r := &delta.Renewal{NewLockedWeight: big.NewInt(1)}

	err := svc.ApplyRenewal(r)
	assert.ErrorContains(t, err, "uint256 overflow")
}

func TestService_ApplyRenewal_QueuedVetNegative(t *testing.T) {
	svc, _, _ := newSvc()
	r := &delta.Renewal{QueuedDecrease: big.NewInt(1)}
	err := svc.ApplyRenewal(r)
	assert.ErrorContains(t, err, "queued-stake uint256 cannot be negative")
}

func TestService_ApplyRenewal_QueuedWeightNegative(t *testing.T) {
	svc, _, _ := newSvc()
	r := &delta.Renewal{QueuedDecreaseWeight: big.NewInt(1)}
	err := svc.ApplyRenewal(r)
	assert.ErrorContains(t, err, "queued-weight uint256 cannot be negative")
}

func TestService_ApplyExit_Negative(t *testing.T) {
	svc, _, _ := newSvc()
	err := svc.ApplyExit(&delta.Exit{ExitedTVL: big.NewInt(1)})
	assert.ErrorContains(t, err, "total-stake uint256 cannot be negative")

	err = svc.ApplyExit(&delta.Exit{ExitedTVLWeight: big.NewInt(1)})
	assert.ErrorContains(t, err, "total-weight uint256 cannot be negative")

	err = svc.ApplyExit(&delta.Exit{QueuedDecrease: big.NewInt(1)})
	assert.ErrorContains(t, err, "queued-stake uint256 cannot be negative")

	err = svc.ApplyExit(&delta.Exit{QueuedDecreaseWeight: big.NewInt(1)})
	assert.ErrorContains(t, err, "queued-weight uint256 cannot be negative")
}
