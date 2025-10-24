// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// #nosec G404
package staker

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestSyncPOS_StillOnPoA(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(120)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_TransitionBlock_NotActive(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(150)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_TransitionBlock_WithValidators(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)

	param.Set(thor.KeyMaxBlockProposers, big.NewInt(3))

	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := &testStaker{
		Staker: New(stakerAddr, st, param, nil),
		addr:   stakerAddr,
		state:  st,
	}

	validator1 := thor.BytesToAddress([]byte("validator1"))
	endorser1 := thor.BytesToAddress([]byte("endorser1"))
	stake := uint64(25_000_000)

	err := staker.AddValidation(validator1, endorser1, uint32(360)*24*15, stake)
	require.NoError(t, err)

	validator2 := thor.BytesToAddress([]byte("validator2"))
	endorser2 := thor.BytesToAddress([]byte("endorser2"))

	err = staker.AddValidation(validator2, endorser2, uint32(360)*24*15, stake)
	require.NoError(t, err)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 10,
	}
	hayabusaTP := uint32(10)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(180)
	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.True(t, status.Active)
	assert.True(t, status.Updates)
}

func TestSyncPOS_TransitionBlock_ZeroTransitionPeriod(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(0)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(100)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_AlreadyActive_NoTransition(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)

	param.Set(thor.KeyMaxBlockProposers, big.NewInt(3))

	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := &testStaker{
		Staker: New(stakerAddr, st, param, nil),
		addr:   stakerAddr,
		state:  st,
	}

	validator1 := thor.BytesToAddress([]byte("validator1"))
	endorser1 := thor.BytesToAddress([]byte("endorser1"))
	stake := uint64(25_000_000)

	err := staker.AddValidation(validator1, endorser1, uint32(360)*24*15, stake)
	require.NoError(t, err)

	validator2 := thor.BytesToAddress([]byte("validator2"))
	endorser2 := thor.BytesToAddress([]byte("endorser2"))

	err = staker.AddValidation(validator2, endorser2, uint32(360)*24*15, stake)
	require.NoError(t, err)

	transitioned, err := staker.transition(0)
	require.NoError(t, err)
	assert.True(t, transitioned)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(200)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.True(t, status.Active)
}

func TestSyncPOS_TransitionBlock_TransitionFails(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)

	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(150)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_NotTransitionBlock(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(175)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_IsPoSActiveError(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(150)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_TransitionError(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(150)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_HousekeepError(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)

	param.Set(thor.KeyMaxBlockProposers, big.NewInt(3))

	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := &testStaker{
		Staker: New(stakerAddr, st, param, nil),
		addr:   stakerAddr,
		state:  st,
	}

	validator1 := thor.BytesToAddress([]byte("validator1"))
	endorser1 := thor.BytesToAddress([]byte("endorser1"))
	stake := uint64(25_000_000)

	err := staker.AddValidation(validator1, endorser1, uint32(360)*24*15, stake)
	require.NoError(t, err)

	validator2 := thor.BytesToAddress([]byte("validator2"))
	endorser2 := thor.BytesToAddress([]byte("endorser2"))

	err = staker.AddValidation(validator2, endorser2, uint32(360)*24*15, stake)
	require.NoError(t, err)

	transitioned, err := staker.transition(0)
	require.NoError(t, err)
	assert.True(t, transitioned)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(200)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)
	assert.True(t, status.Active)
}

func TestSyncPOS_EdgeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	testCases := []struct {
		name           string
		hayabusa       uint32
		hayabusaTP     uint32
		current        uint32
		expectedActive bool
		description    string
	}{
		{
			name:           "exactly_at_fork",
			hayabusa:       100,
			hayabusaTP:     0,
			current:        100,
			expectedActive: false,
			description:    "Should not be active exactly at fork block with zero transition period",
		},
		{
			name:           "one_block_before_transition",
			hayabusa:       100,
			hayabusaTP:     50,
			current:        149,
			expectedActive: false,
			description:    "Should not be active one block before transition",
		},
		{
			name:           "one_block_after_transition",
			hayabusa:       100,
			hayabusaTP:     50,
			current:        151,
			expectedActive: false,
			description:    "Should not be active one block after transition (not transition block)",
		},
		{
			name:           "next_transition_block",
			hayabusa:       100,
			hayabusaTP:     50,
			current:        200,
			expectedActive: false,
			description:    "Should not be active at next transition block (no validators)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			forkConfig := &thor.ForkConfig{
				HAYABUSA: tc.hayabusa,
			}
			thor.SetConfig(thor.Config{HayabusaTP: &tc.hayabusaTP})

			status, err := staker.SyncPOS(forkConfig, tc.current)

			require.NoError(t, err)
			assert.Equal(t, tc.expectedActive, status.Active, tc.description)
		})
	}
}

func TestSyncPOS_StatusFields(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	staker := New(thor.BytesToAddress([]byte("stkr")), st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 100,
	}
	hayabusaTP := uint32(50)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(120)

	status, err := staker.SyncPOS(forkConfig, current)

	require.NoError(t, err)

	assert.False(t, status.Active)
	assert.False(t, status.Updates)
}

func TestSyncPOS_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	paramsAddr := thor.BytesToAddress([]byte("params"))
	param := params.New(thor.BytesToAddress([]byte("params")), st)
	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := New(stakerAddr, st, param, nil)

	forkConfig := &thor.ForkConfig{
		HAYABUSA: 10,
	}
	hayabusaTP := uint32(10)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	current := uint32(180)

	st.SetRawStorage(paramsAddr, thor.KeyMaxBlockProposers, rlp.RawValue{0xFF})
	_, err := staker.SyncPOS(forkConfig, current)
	require.Error(t, err)

	st.SetRawStorage(paramsAddr, thor.KeyMaxBlockProposers, rlp.RawValue{0x2})
	slotActiveGroupSize := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	st.SetRawStorage(stakerAddr, slotActiveGroupSize, rlp.RawValue{0xFF})

	_, err = staker.SyncPOS(forkConfig, current)
	require.Error(t, err)
}

func TestToVET(t *testing.T) {
	weiValue := big.NewInt(-1)
	_, err := ToVET(weiValue)
	assert.ErrorContains(t, err, "wei amount cannot be negative")

	_, err = ToVET(nil)
	assert.ErrorContains(t, err, "wei amount cannot be nil")

	weiValue = big.NewInt(0).SetUint64(math.MaxUint64)
	vetValue, err := ToVET(weiValue)
	assert.NoError(t, err)
	assert.Equal(t, uint64(math.MaxUint64)/1e18, vetValue)

	weiValue = big.NewInt(0).SetUint64(math.MaxUint64)
	weiValue = big.NewInt(0).Mul(weiValue, big.NewInt(1e18))
	vetValue, err = ToVET(weiValue)
	assert.NoError(t, err)
	assert.Equal(t, uint64(math.MaxUint64), vetValue)

	weiValue = big.NewInt(0).Add(weiValue, big.NewInt(1e18))
	_, err = ToVET(weiValue)
	assert.ErrorContains(t, err, "wei amount too large")
}
