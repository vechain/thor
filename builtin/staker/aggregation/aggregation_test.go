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

	assert.True(t, agg.Locked.VET == 0 && agg.Exiting.VET == 0 && agg.Pending.VET == 0)

	agg.Locked.VET = uint64(10000)
	agg.Locked.Weight = uint64(20000)
	agg.Pending.VET = uint64(9000)
	agg.Pending.Weight = uint64(18000)
	agg.Exiting.VET = uint64(8000)
	agg.Exiting.Weight = uint64(16000)

	aggNextPeriodTVL, err := agg.NextPeriodTVL()
	assert.NoError(t, err)
	assert.Equal(t, uint64(11000), aggNextPeriodTVL) // 10000+9000-8000

	renewal, err := agg.renew()
	assert.NoError(t, err)

	assert.Equal(t, uint64(9000), renewal.LockedIncrease.VET)
	assert.Equal(t, uint64(18000), renewal.LockedIncrease.Weight)
	assert.Equal(t, uint64(8000), renewal.LockedDecrease.VET)
	assert.Equal(t, uint64(16000), renewal.LockedDecrease.Weight)

	assert.Equal(t, uint64(11000), agg.Locked.VET)
	assert.Equal(t, uint64(22000), agg.Locked.Weight)
	assert.Equal(t, uint64(0), agg.Pending.VET)
	assert.Equal(t, uint64(0), agg.Pending.Weight)
	assert.Equal(t, uint64(0), agg.Exiting.VET)
	assert.Equal(t, uint64(0), agg.Exiting.Weight)

	agg.Pending.VET = uint64(7000)
	agg.Pending.Weight = uint64(14000)

	exit := agg.exit()
	assert.Equal(t, uint64(11000), exit.ExitedTVL.VET)
	assert.Equal(t, uint64(22000), exit.ExitedTVL.Weight)
	assert.Equal(t, uint64(7000), exit.QueuedDecrease)

	assert.Equal(t, uint64(0), agg.Locked.VET)
	assert.Equal(t, uint64(0), agg.Locked.Weight)
	assert.Equal(t, uint64(0), agg.Pending.VET)
	assert.Equal(t, uint64(0), agg.Pending.Weight)
	assert.Equal(t, uint64(0), agg.Exiting.VET)
	assert.Equal(t, uint64(0), agg.Exiting.Weight)
}
