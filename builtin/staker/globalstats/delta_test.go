// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package globalstats

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/staker/stakes"
)

func TestNewRenewal_Defaults(t *testing.T) {
	r := NewRenewal()
	assert.Equal(t, uint64(0), r.LockedIncrease.VET)
	assert.Equal(t, uint64(0), r.LockedIncrease.Weight)
	assert.Equal(t, uint64(0), r.LockedDecrease.VET)
	assert.Equal(t, uint64(0), r.LockedDecrease.Weight)
}

func TestRenewal_Add(t *testing.T) {
	base := &Renewal{
		LockedIncrease: stakes.NewWeightedStake(10, 20),
		LockedDecrease: stakes.NewWeightedStake(30, 40),
		QueuedDecrease: 50,
	}
	inc := &Renewal{
		LockedIncrease: stakes.NewWeightedStake(1, 2),
		LockedDecrease: stakes.NewWeightedStake(3, 4),
		QueuedDecrease: 1,
	}

	got := base.Add(inc)
	assert.Same(t, base, got)
	assert.Equal(t, uint64(11), got.LockedIncrease.VET)
	assert.Equal(t, uint64(22), got.LockedIncrease.Weight)
	assert.Equal(t, uint64(33), got.LockedDecrease.VET)
	assert.Equal(t, uint64(44), got.LockedDecrease.Weight)
	assert.Equal(t, uint64(51), got.QueuedDecrease)
}

func TestRenewal_Add_Nil(t *testing.T) {
	base := &Renewal{
		LockedIncrease: stakes.NewWeightedStake(5, 6),
		LockedDecrease: stakes.NewWeightedStake(7, 8),
	}
	got := base.Add(nil)
	assert.Same(t, base, got)
	assert.Equal(t, uint64(5), got.LockedIncrease.VET)
	assert.Equal(t, uint64(6), got.LockedIncrease.Weight)
	assert.Equal(t, uint64(7), got.LockedDecrease.VET)
	assert.Equal(t, uint64(8), got.LockedDecrease.Weight)
}

func TestExit_Add(t *testing.T) {
	base := &Exit{
		ExitedTVL:      stakes.NewWeightedStake(100, 200),
		QueuedDecrease: 300,
	}
	inc := &Exit{
		ExitedTVL:      stakes.NewWeightedStake(1, 2),
		QueuedDecrease: 3,
	}

	got := base.Add(inc)
	assert.Same(t, base, got)
	assert.Equal(t, uint64(101), got.ExitedTVL.VET)
	assert.Equal(t, uint64(202), got.ExitedTVL.Weight)
	assert.Equal(t, uint64(303), got.QueuedDecrease)
}

func TestExit_Add_Nil(t *testing.T) {
	base := &Exit{
		ExitedTVL:      stakes.NewWeightedStake(10, 20),
		QueuedDecrease: 30,
	}
	got := base.Add(nil)
	assert.Same(t, base, got)
	assert.Equal(t, uint64(10), got.ExitedTVL.VET)
	assert.Equal(t, uint64(20), got.ExitedTVL.Weight)
	assert.Equal(t, uint64(30), got.QueuedDecrease)
}
