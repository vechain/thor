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

type testValidator struct {
	status    validation.Status
	iteration uint32
}

func (s *testValidator) Status() validation.Status {
	return s.status
}

func (s *testValidator) CurrentIteration(_ uint32) (uint32, error) {
	return s.iteration, nil
}

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

	val := &testValidator{}

	started, err := del.Started(val, 10)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err := del.Ended(val, 10)
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

	val.status = validation.StatusQueued
	started, err = del.Started(val, 10)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err = del.Ended(val, 10)
	assert.NoError(t, err)
	assert.False(t, ended)

	val.status = validation.StatusActive
	del.FirstIteration = 4
	started, err = del.Started(val, 4)
	assert.NoError(t, err)
	assert.False(t, started)
	ended, err = del.Ended(val, 4)
	assert.NoError(t, err)
	assert.False(t, ended)

	val.iteration = 4
	started, err = del.Started(val, 4)
	assert.NoError(t, err)
	assert.True(t, started)

	val.status = validation.StatusExit
	val.iteration = 5
	ended, err = del.Ended(val, 6)
	assert.NoError(t, err)
	assert.True(t, ended)

	val.status = validation.StatusActive
	lastIter := uint32(5)
	del.LastIteration = &lastIter
	val.iteration = 2
	ended, err = del.Ended(val, 5)
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

	val := testValidator{
		status: validation.StatusActive,
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

	val.status = validation.StatusExit
	isLocked, err = del.IsLocked(&val, 2)
	assert.NoError(t, err)
	assert.False(t, isLocked)
}
