// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package validation

import (
	"math/big"
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
	LockedVET:          big.NewInt(1000),
	PendingUnlockVET:   big.NewInt(900),
	QueuedVET:          big.NewInt(800),
	CooldownVET:        big.NewInt(700),
	WithdrawableVET:    big.NewInt(600),
	Weight:             big.NewInt(1000),
}

func TestValidation_Totals(t *testing.T) {
	agg := aggregation.Aggregation{
		LockedVET:     big.NewInt(500),
		LockedWeight:  big.NewInt(1000),
		PendingVET:    big.NewInt(400),
		PendingWeight: big.NewInt(800),
		ExitingVET:    big.NewInt(300),
		ExitingWeight: big.NewInt(600),
	}
	totals := baseVal.Totals(&agg)
	assert.Equal(t, big.NewInt(1500), totals.TotalLockedStake)
	assert.Equal(t, big.NewInt(1000), totals.TotalLockedWeight)
	assert.Equal(t, big.NewInt(1200), totals.TotalQueuedStake)
	assert.Equal(t, big.NewInt(1600), totals.TotalQueuedWeight)
	assert.Equal(t, big.NewInt(1200), totals.TotalExitingStake)
	assert.Equal(t, big.NewInt(1500), totals.TotalExitingWeight)

	exitBlock := uint32(2)
	val := baseVal
	val.ExitBlock = &exitBlock
	totals = val.Totals(&agg)
	assert.Equal(t, big.NewInt(1500), totals.TotalLockedStake)
	assert.Equal(t, big.NewInt(1000), totals.TotalLockedWeight)
	assert.Equal(t, big.NewInt(1200), totals.TotalQueuedStake)
	assert.Equal(t, big.NewInt(1600), totals.TotalQueuedWeight)
	assert.Equal(t, big.NewInt(1500), totals.TotalExitingStake)
	assert.Equal(t, big.NewInt(1000), totals.TotalExitingWeight)
}

func TestValidation_IsPeriodEnd(t *testing.T) {
	assert.True(t, baseVal.IsPeriodEnd(5))
	assert.False(t, baseVal.IsPeriodEnd(6))
}

func TestValidation_NextPeriodTVL(t *testing.T) {
	assert.Equal(t, big.NewInt(900), baseVal.NextPeriodTVL())
}

func TestValidation_Exit(t *testing.T) {
	val := baseVal
	delta := val.exit(nil)
	assert.Equal(t, StatusExit, val.Status)
	assert.Equal(t, big.NewInt(1000), val.CooldownVET)
	assert.Equal(t, big.NewInt(0), val.LockedVET)
	assert.Equal(t, big.NewInt(0), val.PendingUnlockVET)
	assert.Equal(t, big.NewInt(0), val.Weight)

	assert.Equal(t, big.NewInt(1000), delta.ExitedTVL)
	assert.Equal(t, big.NewInt(1000), delta.ExitedTVLWeight)
	assert.Equal(t, big.NewInt(800), delta.QueuedDecrease)
	assert.Equal(t, big.NewInt(800), delta.QueuedDecreaseWeight)
}
