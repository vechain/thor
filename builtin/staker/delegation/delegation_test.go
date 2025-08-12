// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package delegation

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

func TestDelegation(t *testing.T) {
	del := Delegation{
		Validation:     thor.Address{},
		Stake:          big.NewInt(0),
		Multiplier:     0,
		LastIteration:  nil,
		FirstIteration: 0,
	}

	assert.True(t, del.IsEmpty())
	wStake := del.WeightedStake()
	assert.Equal(t, big.NewInt(0), wStake.VET())
	assert.Equal(t, big.NewInt(0), wStake.Weight())

	val := validation.Validation{
		Endorsor:           thor.Address{},
		Period:             0,
		CompleteIterations: 0,
		Status:             0,
		Online:             false,
		StartBlock:         0,
		ExitBlock:          nil,
		LockedVET:          nil,
		PendingUnlockVET:   nil,
		QueuedVET:          nil,
		CooldownVET:        nil,
		WithdrawableVET:    nil,
		Weight:             nil,
	}

	assert.False(t, del.Started(&val))
	assert.False(t, del.Ended(&val))

	del = Delegation{
		Validation:     thor.Address{},
		Stake:          big.NewInt(1000),
		Multiplier:     200,
		LastIteration:  nil,
		FirstIteration: 0,
	}

	assert.False(t, del.IsEmpty())
	wStake = del.WeightedStake()
	assert.Equal(t, big.NewInt(1000), wStake.VET())
	assert.Equal(t, big.NewInt(2000), wStake.Weight())

	val.Status = validation.StatusQueued
	assert.False(t, del.Started(&val))
	assert.False(t, del.Ended(&val))

	val.Status = validation.StatusActive
	val.CompleteIterations = 2
	del.FirstIteration = 4
	assert.False(t, del.Started(&val))
	assert.False(t, del.Ended(&val))

	val.CompleteIterations = 3
	assert.True(t, del.Started(&val))

	val.Status = validation.StatusExit
	val.CompleteIterations = 4
	assert.True(t, del.Ended(&val))

	val.Status = validation.StatusActive
	lastIter := uint32(5)
	del.LastIteration = &lastIter
	assert.False(t, del.Ended(&val))
}
