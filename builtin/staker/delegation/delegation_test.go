// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package delegation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

func TestDelegation(t *testing.T) {
	del := Delegation{
		Validation:     thor.Address{},
		Stake:          0,
		Multiplier:     0,
		LastIteration:  nil,
		FirstIteration: 0,
	}

	wStake := del.WeightedStake()
	assert.Equal(t, uint64(0), wStake.VET)
	assert.Equal(t, uint64(0), wStake.Weight)
	offlineBlock := uint32(0)

	val := validation.Validation{
		Endorser:         thor.Address{},
		Period:           2,
		CompletedPeriods: 0,
		Status:           0,
		OfflineBlock:     &offlineBlock,
		StartBlock:       0,
		ExitBlock:        nil,
		LockedVET:        0,
		PendingUnlockVET: 0,
		QueuedVET:        0,
		CooldownVET:      0,
		WithdrawableVET:  0,
		Weight:           0,
	}

	started, err := del.Started(&val, 10)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err := del.Ended(&val, 10)
	assert.NoError(t, err)
	assert.False(t, ended)

	del = Delegation{
		Validation:     thor.Address{},
		Stake:          1000,
		Multiplier:     200,
		LastIteration:  nil,
		FirstIteration: 0,
	}

	wStake = del.WeightedStake()
	assert.Equal(t, uint64(1000), wStake.VET)
	assert.Equal(t, uint64(2000), wStake.Weight)

	val.Status = validation.StatusUnknown
	started, err = del.Started(&val, 10)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err = del.Ended(&val, 10)
	assert.NoError(t, err)
	assert.False(t, ended)

	val.Status = validation.StatusQueued
	started, err = del.Started(&val, 10)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err = del.Ended(&val, 10)
	assert.NoError(t, err)
	assert.False(t, ended)

	val.Status = validation.StatusActive
	del.FirstIteration = 4
	started, err = del.Started(&val, 4)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err = del.Ended(&val, 4)
	assert.NoError(t, err)
	assert.False(t, ended)

	val.Period = 1
	started, err = del.Started(&val, 4)
	assert.NoError(t, err)
	assert.True(t, started)

	val.Status = validation.StatusExit
	val.CompletedPeriods = 4
	ended, err = del.Ended(&val, 6)
	assert.NoError(t, err)
	assert.True(t, ended)

	val.Status = validation.StatusActive
	lastIter := uint32(5)
	del.LastIteration = &lastIter
	val.Period = 2
	ended, err = del.Ended(&val, 5)
	assert.NoError(t, err)
	assert.False(t, ended)
}

func TestDelegation_IsLocked(t *testing.T) {
	del := Delegation{
		Validation:     thor.Address{},
		Stake:          0,
		Multiplier:     0,
		LastIteration:  nil,
		FirstIteration: 0,
	}

	val := validation.Validation{
		Endorser:         thor.Address{},
		Period:           1,
		CompletedPeriods: 0,
		Status:           validation.StatusActive,
		OfflineBlock:     nil,
		StartBlock:       0,
		ExitBlock:        nil,
		LockedVET:        0,
		PendingUnlockVET: 0,
		QueuedVET:        0,
		CooldownVET:      0,
		WithdrawableVET:  0,
		Weight:           0,
	}

	isLocked, err := del.IsLocked(&val, 1)
	assert.NoError(t, err)
	assert.False(t, isLocked)

	isLocked, err = del.IsLocked(&val, 2)
	assert.NoError(t, err)
	assert.False(t, isLocked)

	del.Stake = 1000
	isLocked, err = del.IsLocked(&val, 3)
	assert.NoError(t, err)
	assert.True(t, isLocked)

	val.Status = validation.StatusExit
	isLocked, err = del.IsLocked(&val, 2)
	assert.NoError(t, err)
	assert.False(t, isLocked)
}

func TestDelegation_ErrorPaths_Matrix(t *testing.T) {
	currentBlock := uint32(10)
	v := &validation.Validation{
		Status:     validation.StatusActive,
		Period:     0,
		StartBlock: 0,
	}

	type args struct {
		delegation Delegation
	}
	tests := []struct {
		name     string
		testFunc func(*Delegation, *validation.Validation, uint32) (interface{}, error)
		args     args
	}{
		{
			name: "Started returns error (CurrentIteration error)",
			testFunc: func(d *Delegation, v *validation.Validation, block uint32) (interface{}, error) {
				return d.Started(v, block)
			},
			args: args{
				delegation: Delegation{FirstIteration: 1},
			},
		},
		{
			name: "Ended returns error (Started error)",
			testFunc: func(d *Delegation, v *validation.Validation, block uint32) (interface{}, error) {
				return d.Ended(v, block)
			},
			args: args{
				delegation: Delegation{FirstIteration: 1, LastIteration: new(uint32)},
			},
		},
		{
			name: "IsLocked returns error (Started error)",
			testFunc: func(d *Delegation, v *validation.Validation, block uint32) (interface{}, error) {
				return d.IsLocked(v, block)
			},
			args: args{
				delegation: Delegation{FirstIteration: 1, Stake: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.testFunc(&tt.args.delegation, v, currentBlock)
			assert.Error(t, err)
		})
	}
}
