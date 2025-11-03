// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/stakes"

	"github.com/vechain/thor/v2/thor"
)

var baseVal = Validation{
	body: &body{
		Endorser:         thor.Address{},
		Beneficiary:      nil,
		Period:           5,
		CompletedPeriods: 0,
		Status:           StatusActive,
		StartBlock:       0,
		ExitBlock:        nil,
		OfflineBlock:     nil,
		LockedVET:        1000,
		PendingUnlockVET: 900,
		QueuedVET:        800,
		CooldownVET:      700,
		WithdrawableVET:  600,
		Weight:           1000,
	},
}

type testAggregation struct {
	queued  *stakes.WeightedStake
	locked  *stakes.WeightedStake
	exiting *stakes.WeightedStake
}

func (ta *testAggregation) Locked() *stakes.WeightedStake {
	return ta.locked
}

func (ta *testAggregation) Pending() *stakes.WeightedStake {
	return ta.queued
}

func (ta *testAggregation) Exiting() *stakes.WeightedStake {
	return ta.exiting
}

func (ta *testAggregation) NextPeriodTVL() (uint64, error) {
	return ta.locked.VET + ta.queued.VET - ta.exiting.VET, nil
}

func TestValidation_Totals(t *testing.T) {
	agg := &testAggregation{
		locked:  stakes.NewWeightedStake(500, 1000),
		queued:  stakes.NewWeightedStake(400, 800),
		exiting: stakes.NewWeightedStake(300, 600),
	}
	totals, err := baseVal.Totals(agg)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1500), totals.TotalLockedStake)
	assert.Equal(t, uint64(1000), totals.TotalLockedWeight)
	assert.Equal(t, uint64(1200), totals.TotalQueuedStake)
	assert.Equal(t, uint64(1200), totals.TotalExitingStake)
	assert.Equal(t, uint64(3000), totals.NextPeriodWeight) // (1000-900+800)*2 + (1000+800-600)

	exitBlock := uint32(2)
	val := baseVal
	val.body.ExitBlock = &exitBlock
	totals, err = val.Totals(agg)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1500), totals.TotalLockedStake)
	assert.Equal(t, uint64(1000), totals.TotalLockedWeight)
	assert.Equal(t, uint64(1200), totals.TotalQueuedStake)
	assert.Equal(t, uint64(1500), totals.TotalExitingStake)
	assert.Equal(t, uint64(0), totals.NextPeriodWeight)
}

func TestValidation_IsPeriodEnd(t *testing.T) {
	assert.True(t, baseVal.IsPeriodEnd(5))
	assert.False(t, baseVal.IsPeriodEnd(6))
}

func TestValidation_NextPeriodTVL(t *testing.T) {
	valNextPeriodTVL, err := baseVal.NextPeriodTVL()
	assert.NoError(t, err)
	assert.Equal(t, uint64(900), valNextPeriodTVL)
}

func TestValidation_Exit(t *testing.T) {
	val := baseVal
	delta := val.exit()
	assert.Equal(t, StatusExit, val.Status())
	assert.Equal(t, uint64(1000), val.CooldownVET())
	assert.Equal(t, uint64(0), val.LockedVET())
	assert.Equal(t, uint64(0), val.PendingUnlockVET())
	assert.Equal(t, uint64(0), val.Weight())

	assert.Equal(t, uint64(1000), delta.ExitedTVL.VET)
	assert.Equal(t, uint64(1000), delta.ExitedTVL.Weight)
	assert.Equal(t, uint64(800), delta.QueuedDecrease)
}

func TestIterations(t *testing.T) {
	val := Validation{
		body: &body{
			Status:     StatusQueued,
			Period:     5,
			StartBlock: 0,
		},
	}
	current, err := val.CurrentIteration(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), current)
	current, err = val.CompletedIterations(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), current)

	val.body.Status = StatusUnknown
	current, err = val.CurrentIteration(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), current)
	current, err = val.CompletedIterations(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), current)

	// change to exit
	val.body.Status = StatusExit
	current, err = val.CompletedIterations(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), current)

	val.body.CompletedPeriods = 1
	current, err = val.CompletedIterations(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), current)

	current, err = val.CurrentIteration(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), current)

	val.body.Status = StatusActive
	val.body.CompletedPeriods = 0

	current, err = val.CurrentIteration(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), current)

	current, err = val.CompletedIterations(10)
	assert.NoError(t, err)
	assert.Equal(t, uint32(2), current)

	// signaled exit in period 3, block 13
	val.body.CompletedPeriods = 3

	current, err = val.CurrentIteration(13)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), current)

	current, err = val.CompletedIterations(13)
	assert.NoError(t, err)
	assert.Equal(t, uint32(2), current)

	// last period stayed more than 1 period
	current, err = val.CompletedIterations(18)
	assert.NoError(t, err)
	assert.Equal(t, uint32(2), current)

	// exited
	val.body.Status = StatusExit
	current, err = val.CompletedIterations(18)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), current)

	current, err = val.CurrentIteration(18)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), current)

	// status exited stopped at last period
	current, err = val.CurrentIteration(200)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3), current)
}
