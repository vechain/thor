// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package staker

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestTransition(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	param := params.New(thor.BytesToAddress([]byte("params")), st)

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(2)))
	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := New(stakerAddr, st, param, nil)

	isExecuted, err := staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	node1 := datagen.RandAddress()
	stake := RandomStake()
	err = staker.AddValidation(node1, node1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	node2 := datagen.RandAddress()
	err = staker.AddValidation(node2, node2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	staker.params.Set(thor.KeyMaxBlockProposers, big.NewInt(0))

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	staker.params.Set(thor.KeyMaxBlockProposers, big.NewInt(2))

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.True(t, isExecuted)

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	activeCountSlot := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	st.SetRawStorage(stakerAddr, activeCountSlot, rlp.RawValue{0xFF})
	isExecuted, err = staker.transition(thor.EpochLength())
	assert.Error(t, err)
	assert.False(t, isExecuted)

	queuedCountSlot := thor.BytesToBytes32([]byte(("validations-queued-group-size")))
	st.SetRawStorage(stakerAddr, activeCountSlot, rlp.RawValue{0x0})
	st.SetRawStorage(stakerAddr, queuedCountSlot, rlp.RawValue{0xFF})
}

func TestStaker_TransitionPeriodBalanceCheck(t *testing.T) {
	fc := &thor.ForkConfig{
		HAYABUSA:    10,
		HAYABUSA_TP: 20,
	}

	endorsement := big.NewInt(1000)
	endorser := datagen.RandAddress()
	master := datagen.RandAddress()

	tests := []struct {
		name         string
		currentBlock uint32
		preTestHook  func(staker *Staker)
		ok           bool
	}{
		{
			name:         "before hayabusa, validator has greater than funds",
			currentBlock: 5,
			preTestHook: func(staker *Staker) {
				assert.NoError(t, staker.state.SetBalance(endorser, big.NewInt(2000)))
			},
			ok: true,
		},
		{
			name:         "before hayabusa, validator funds are too low",
			currentBlock: 5,
			preTestHook: func(staker *Staker) {
				assert.NoError(t, staker.state.SetBalance(endorser, big.NewInt(500)))
			},
			ok: false,
		},
		{
			name:         "before hayabusa, validator has exactly enough funds",
			currentBlock: 5,
			preTestHook: func(staker *Staker) {
				assert.NoError(t, staker.state.SetBalance(endorser, big.NewInt(1000)))
			},
			ok: true,
		},
		{
			name:         "during transition period, validator has not staked, has enough funds",
			currentBlock: 15,
			preTestHook: func(staker *Staker) {
				assert.NoError(t, staker.state.SetBalance(endorser, big.NewInt(2000)))
			},
			ok: true,
		},
		{
			name:         "during transition period, validator has not staked, has insufficient funds",
			currentBlock: 15,
			preTestHook: func(staker *Staker) {
				assert.NoError(t, staker.state.SetBalance(endorser, big.NewInt(500)))
			},
			ok: false,
		},
		{
			name:         "during transition period, validator has staked",
			currentBlock: 15,
			preTestHook: func(staker *Staker) {
				assert.NoError(t, staker.AddValidation(master, endorser, thor.MediumStakingPeriod(), MinStake))
			},
			ok: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staker, _ := newStaker(t, 1, 1, false)
			tt.preTestHook(staker)
			balanceCheck := staker.TransitionPeriodBalanceCheck(fc, tt.currentBlock, endorsement)
			ok, err := balanceCheck(master, endorser)
			assert.NoError(t, err)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
