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

	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

func TestTransition(t *testing.T) {
	staker := newTest(t).SetMBP(2)

	isExecuted, err := staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	node1 := datagen.RandAddress()
	stake := RandomStake()
	staker.AddValidation(node1, node1, uint32(360)*24*15, stake)

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	node2 := datagen.RandAddress()
	staker.AddValidation(node2, node2, uint32(360)*24*15, stake)

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

	st := staker.State()
	stakerAddr := staker.Address()

	activeCountSlot := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	st.SetRawStorage(stakerAddr, activeCountSlot, rlp.RawValue{0xFF})
	isExecuted, err = staker.transition(thor.EpochLength())
	assert.Error(t, err)
	assert.False(t, isExecuted)

	queuedCountSlot := thor.BytesToBytes32([]byte(("validations-queued-group-size")))
	st.SetRawStorage(stakerAddr, activeCountSlot, rlp.RawValue{0x0})
	st.SetRawStorage(stakerAddr, queuedCountSlot, rlp.RawValue{0xFF})
}

func TestTransitionWithPreExistingVET(t *testing.T) {
	tests := []struct {
		name        string
		expectError bool
		addBalance  bool // true = add VET, false = remove VET
	}{
		{
			name:        "Extra VET should pass",
			expectError: false,
			addBalance:  true,
		},
		{
			name:        "Insufficient VET should error",
			expectError: true,
			addBalance:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staker := newTest(t).SetMBP(2).Fill(2)

			// Add validators
			node1 := datagen.RandAddress()
			node2 := datagen.RandAddress()
			stake := RandomStake()

			staker.AddValidation(node1, node1, uint32(360)*24*15, stake)
			staker.AddValidation(node2, node2, uint32(360)*24*15, stake)

			// Modify balance
			currentBalance, err := staker.State().GetBalance(staker.Address())
			assert.NoError(t, err)

			changeAmount := ToWei(1000000) // 1M VET
			var newBalance *big.Int
			if tt.addBalance {
				newBalance = big.NewInt(0).Add(currentBalance, changeAmount)
			} else {
				newBalance = big.NewInt(0).Sub(currentBalance, changeAmount)
				if newBalance.Sign() < 0 {
					newBalance = big.NewInt(0)
				}
			}
			assert.NoError(t, staker.state.SetBalance(staker.Address(), newBalance))

			// Test ContractBalanceCheck
			err = staker.ContractBalanceCheck(0)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStaker_TransitionPeriodBalanceCheck(t *testing.T) {
	fc := &thor.ForkConfig{
		HAYABUSA: 10,
	}
	tp := uint32(10)
	thor.SetConfig(thor.Config{HayabusaTP: &tp})
	bigE18 := big.NewInt(1e18)

	endorsement := big.NewInt(1000)
	endorser := datagen.RandAddress()
	master := datagen.RandAddress()

	tests := []struct {
		name         string
		currentBlock uint32
		preTestHook  func(staker *StakerTest)
		ok           bool
	}{
		{
			name:         "before hayabusa, validator has greater than funds",
			currentBlock: 5,
			preTestHook: func(staker *StakerTest) {
				assert.NoError(t, staker.State().SetBalance(endorser, big.NewInt(2000)))
			},
			ok: true,
		},
		{
			name:         "before hayabusa, validator funds are too low",
			currentBlock: 5,
			preTestHook: func(staker *StakerTest) {
				assert.NoError(t, staker.State().SetBalance(endorser, big.NewInt(500)))
			},
			ok: false,
		},
		{
			name:         "before hayabusa, validator has exactly enough funds",
			currentBlock: 5,
			preTestHook: func(staker *StakerTest) {
				assert.NoError(t, staker.State().SetBalance(endorser, big.NewInt(1000)))
			},
			ok: true,
		},
		{
			name:         "during transition period, validator has not staked, has enough funds",
			currentBlock: 15,
			preTestHook: func(staker *StakerTest) {
				assert.NoError(t, staker.State().SetBalance(endorser, big.NewInt(2000)))
			},
			ok: true,
		},
		{
			name:         "during transition period, validator has not staked, has insufficient funds",
			currentBlock: 15,
			preTestHook: func(staker *StakerTest) {
				assert.NoError(t, staker.State().SetBalance(endorser, big.NewInt(500)))
			},
			ok: false,
		},
		{
			name:         "during transition period, validator has staked",
			currentBlock: 15,
			preTestHook: func(staker *StakerTest) {
				stake := big.NewInt(0).Div(MinStake, bigE18).Uint64()
				staker.AddValidation(master, endorser, thor.MediumStakingPeriod(), stake)
			},
			ok: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := newTest(t).SetMBP(1).Fill(1).Transition(0)
			tt.preTestHook(test)
			balanceCheck := test.TransitionPeriodBalanceCheck(fc, tt.currentBlock, endorsement)
			ok, err := balanceCheck(master, endorser)
			assert.NoError(t, err)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
