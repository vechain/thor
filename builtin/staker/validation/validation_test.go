// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/thor"
)

var baseVal = Validation{
	Endorser:           thor.Address{},
	Beneficiary:        nil,
	Period:             5,
	CompleteIterations: 0,
	Status:             StatusActive,
	StartBlock:         0,
	ExitBlock:          nil,
	OfflineBlock:       nil,
	LockedVET:          1000,
	PendingUnlockVET:   900,
	QueuedVET:          800,
	CooldownVET:        700,
	WithdrawableVET:    600,
	Weight:             1000,
}

func TestValidation_Totals(t *testing.T) {
	agg := aggregation.Aggregation{
		LockedVET:     500,
		LockedWeight:  1000,
		PendingVET:    400,
		PendingWeight: 800,
		ExitingVET:    300,
		ExitingWeight: 600,
	}
	totals := baseVal.Totals(&agg)
	assert.Equal(t, uint64(1500), totals.TotalLockedStake)
	assert.Equal(t, uint64(1000), totals.TotalLockedWeight)
	assert.Equal(t, uint64(1200), totals.TotalQueuedStake)
	assert.Equal(t, uint64(1200), totals.TotalExitingStake)
	assert.Equal(t, uint64(3000), totals.NextPeriodWeight) // (1000-900+800)*2 + (1000+800-600)

	exitBlock := uint32(2)
	val := baseVal
	val.ExitBlock = &exitBlock
	totals = val.Totals(&agg)
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
	assert.Equal(t, uint64(900), baseVal.NextPeriodTVL())
}

func TestValidation_Exit(t *testing.T) {
	val := baseVal
	delta := val.exit()
	assert.Equal(t, StatusExit, val.Status)
	assert.Equal(t, uint64(1000), val.CooldownVET)
	assert.Equal(t, uint64(0), val.LockedVET)
	assert.Equal(t, uint64(0), val.PendingUnlockVET)
	assert.Equal(t, uint64(0), val.Weight)

	assert.Equal(t, uint64(1000), delta.ExitedTVL.VET)
	assert.Equal(t, uint64(1000), delta.ExitedTVL.Weight)
	assert.Equal(t, uint64(800), delta.QueuedDecrease)
}
