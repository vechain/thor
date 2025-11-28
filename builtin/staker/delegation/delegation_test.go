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
		&body{
			Validation:     thor.Address{},
			Stake:          0,
			Multiplier:     0,
			LastIteration:  nil,
			FirstIteration: 0,
		},
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
		&body{
			Validation:     thor.Address{},
			Stake:          1000,
			Multiplier:     200,
			LastIteration:  nil,
			FirstIteration: 0,
		},
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
	del.body.FirstIteration = 4
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
	del.body.LastIteration = &lastIter
	val.iteration = 2
	ended, err = del.Ended(val, 5)
	assert.NoError(t, err)
	assert.False(t, ended)
}

func TestDelegation_IsLocked(t *testing.T) {
	del := Delegation{
		&body{
			Validation:     thor.Address{},
			Stake:          0,
			Multiplier:     0,
			LastIteration:  nil,
			FirstIteration: 0,
		},
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

	del.body.Stake = 1000
	isLocked, err = del.IsLocked(&val, 3)
	assert.NoError(t, err)
	assert.True(t, isLocked)

	val.status = validation.StatusExit
	isLocked, err = del.IsLocked(&val, 2)
	assert.NoError(t, err)
	assert.False(t, isLocked)
}

func Test_IsLocked(t *testing.T) {
	t.Run("Completed Staking Periods", func(t *testing.T) {
		last := uint32(2)
		d := &Delegation{
			&body{
				FirstIteration: 2,
				LastIteration:  &last,
				Stake:          uint64(1),
				Multiplier:     255,
			},
		}

		v := &testValidator{
			status:    validation.StatusActive,
			iteration: 5,
		}

		stared, err := d.Started(v, 10)
		assert.NoError(t, err)
		assert.True(t, stared, "should not be locked when complete iterations is equal to last iteration")
		ended, err := d.Ended(v, 15)
		assert.NoError(t, err)
		assert.True(t, ended, "should be locked when first is less than current and last is equal to current")
	})

	t.Run("Incomplete Staking Periods", func(t *testing.T) {
		last := uint32(5)
		d := &Delegation{
			&body{
				FirstIteration: 2,
				LastIteration:  &last,
				Stake:          uint64(1),
				Multiplier:     255,
			},
		}

		v := &testValidator{
			status:    validation.StatusActive,
			iteration: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.True(t, started, "should be started when complete iterations is greater than first iteration")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when first is less than current and last is greater than current")
	})

	t.Run("Delegation Not Started", func(t *testing.T) {
		last := uint32(6)
		d := &Delegation{
			&body{
				FirstIteration: 5,
				LastIteration:  &last,
				Stake:          uint64(1),
				Multiplier:     255,
			},
		}

		v := &testValidator{
			status:    validation.StatusActive,
			iteration: 4,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.False(t, started, "should not be started when complete iterations is less than first iteration")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when first is greater than current and last is greater than current")
	})
	t.Run("Staker is Queued", func(t *testing.T) {
		d := &Delegation{
			&body{
				FirstIteration: 1,
				LastIteration:  nil,
				Stake:          uint64(1),
				Multiplier:     255,
			},
		}

		v := &testValidator{
			status:    validation.StatusQueued,
			iteration: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.False(t, started, "should not be started when validation status is queued")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when validation status is queued")
	})

	t.Run("exit block not defined", func(t *testing.T) {
		d := &Delegation{
			&body{
				FirstIteration: 1,
				LastIteration:  nil,
				Stake:          uint64(1),
				Multiplier:     255,
			},
		}

		v := &testValidator{
			status:    validation.StatusActive,
			iteration: 5,
		}

		started, err := d.Started(v, 15)
		assert.NoError(t, err)
		assert.True(t, started, "should be started when first iteration is less than current")
		ended, err := d.Ended(v, 20)
		assert.NoError(t, err)
		assert.False(t, ended, "should not be locked when last iteration is nil and first equals current")
	})
}
