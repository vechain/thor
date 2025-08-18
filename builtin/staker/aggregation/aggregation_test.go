// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package aggregation

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAggregation(t *testing.T) {
	agg := newAggregation()

	assert.True(t, agg.IsEmpty())

	agg.LockedVET = big.NewInt(10000)
	agg.LockedWeight = big.NewInt(20000)
	agg.PendingVET = big.NewInt(9000)
	agg.PendingWeight = big.NewInt(18000)
	agg.ExitingVET = big.NewInt(8000)
	agg.ExitingWeight = big.NewInt(16000)

	assert.Equal(t, big.NewInt(19000), agg.NextPeriodTVL())

	renewal := agg.renew()

	assert.Equal(t, big.NewInt(1000), renewal.NewLockedVET)
	assert.Equal(t, big.NewInt(2000), renewal.NewLockedWeight)
	assert.Equal(t, big.NewInt(9000), renewal.QueuedDecrease)
	assert.Equal(t, big.NewInt(18000), renewal.QueuedDecreaseWeight)

	assert.Equal(t, big.NewInt(11000), agg.LockedVET)
	assert.Equal(t, big.NewInt(22000), agg.LockedWeight)
	assert.Equal(t, big.NewInt(0), agg.PendingVET)
	assert.Equal(t, big.NewInt(0), agg.PendingWeight)
	assert.Equal(t, big.NewInt(0), agg.ExitingVET)
	assert.Equal(t, big.NewInt(0), agg.ExitingWeight)

	agg.PendingVET = big.NewInt(7000)
	agg.PendingWeight = big.NewInt(14000)

	exit := agg.exit()
	assert.Equal(t, big.NewInt(11000), exit.ExitedTVL)
	assert.Equal(t, big.NewInt(22000), exit.ExitedTVLWeight)
	assert.Equal(t, big.NewInt(7000), exit.QueuedDecrease)
	assert.Equal(t, big.NewInt(14000), exit.QueuedDecreaseWeight)

	assert.Equal(t, big.NewInt(0), agg.LockedVET)
	assert.Equal(t, big.NewInt(0), agg.LockedWeight)
	assert.Equal(t, big.NewInt(0), agg.PendingVET)
	assert.Equal(t, big.NewInt(0), agg.PendingWeight)
	assert.Equal(t, big.NewInt(0), agg.ExitingVET)
	assert.Equal(t, big.NewInt(0), agg.ExitingWeight)
}
