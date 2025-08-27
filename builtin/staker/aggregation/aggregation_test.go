// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package aggregation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAggregation(t *testing.T) {
	agg := newAggregation()

	assert.True(t, agg.IsEmpty())

	agg.LockedVET = uint64(10000)
	agg.LockedWeight = uint64(20000)
	agg.PendingVET = uint64(9000)
	agg.PendingWeight = uint64(18000)
	agg.ExitingVET = uint64(8000)
	agg.ExitingWeight = uint64(16000)

	assert.Equal(t, uint64(19000), agg.NextPeriodTVL())

	renewal := agg.renew()

	assert.Equal(t, uint64(9000), renewal.LockedIncrease.VET)
	assert.Equal(t, uint64(18000), renewal.LockedIncrease.Weight)
	assert.Equal(t, uint64(8000), renewal.LockedDecrease.VET)
	assert.Equal(t, uint64(16000), renewal.LockedDecrease.Weight)

	assert.Equal(t, uint64(11000), agg.LockedVET)
	assert.Equal(t, uint64(22000), agg.LockedWeight)
	assert.Equal(t, uint64(0), agg.PendingVET)
	assert.Equal(t, uint64(0), agg.PendingWeight)
	assert.Equal(t, uint64(0), agg.ExitingVET)
	assert.Equal(t, uint64(0), agg.ExitingWeight)

	agg.PendingVET = uint64(7000)
	agg.PendingWeight = uint64(14000)

	exit := agg.exit()
	assert.Equal(t, uint64(11000), exit.ExitedTVL.VET)
	assert.Equal(t, uint64(22000), exit.ExitedTVL.Weight)
	assert.Equal(t, uint64(7000), exit.QueuedDecrease.VET)
	assert.Equal(t, uint64(14000), exit.QueuedDecrease.Weight)

	assert.Equal(t, uint64(0), agg.LockedVET)
	assert.Equal(t, uint64(0), agg.LockedWeight)
	assert.Equal(t, uint64(0), agg.PendingVET)
	assert.Equal(t, uint64(0), agg.PendingWeight)
	assert.Equal(t, uint64(0), agg.ExitingVET)
	assert.Equal(t, uint64(0), agg.ExitingWeight)
}
