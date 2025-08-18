// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package delta

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRenewal_Defaults(t *testing.T) {
	r := NewRenewal()
	assert.Equal(t, big.NewInt(0), r.NewLockedVET)
	assert.Equal(t, big.NewInt(0), r.NewLockedWeight)
	assert.Equal(t, big.NewInt(0), r.QueuedDecrease)
	assert.Equal(t, big.NewInt(0), r.QueuedDecreaseWeight)
}

func TestRenewal_Add(t *testing.T) {
	base := &Renewal{
		NewLockedVET:         big.NewInt(10),
		NewLockedWeight:      big.NewInt(20),
		QueuedDecrease:       big.NewInt(30),
		QueuedDecreaseWeight: big.NewInt(40),
	}
	inc := &Renewal{
		NewLockedVET:         big.NewInt(1),
		NewLockedWeight:      big.NewInt(2),
		QueuedDecrease:       big.NewInt(3),
		QueuedDecreaseWeight: big.NewInt(4),
	}

	got := base.Add(inc)
	assert.Same(t, base, got)
	assert.Equal(t, big.NewInt(11), got.NewLockedVET)
	assert.Equal(t, big.NewInt(22), got.NewLockedWeight)
	assert.Equal(t, big.NewInt(33), got.QueuedDecrease)
	assert.Equal(t, big.NewInt(44), got.QueuedDecreaseWeight)
}

func TestRenewal_Add_Nil(t *testing.T) {
	base := &Renewal{
		NewLockedVET:         big.NewInt(5),
		NewLockedWeight:      big.NewInt(6),
		QueuedDecrease:       big.NewInt(7),
		QueuedDecreaseWeight: big.NewInt(8),
	}
	got := base.Add(nil)
	assert.Same(t, base, got)
	assert.Equal(t, big.NewInt(5), got.NewLockedVET)
	assert.Equal(t, big.NewInt(6), got.NewLockedWeight)
	assert.Equal(t, big.NewInt(7), got.QueuedDecrease)
	assert.Equal(t, big.NewInt(8), got.QueuedDecreaseWeight)
}

func TestExit_Add(t *testing.T) {
	base := &Exit{
		ExitedTVL:            big.NewInt(100),
		ExitedTVLWeight:      big.NewInt(200),
		QueuedDecrease:       big.NewInt(300),
		QueuedDecreaseWeight: big.NewInt(400),
	}
	inc := &Exit{
		ExitedTVL:            big.NewInt(1),
		ExitedTVLWeight:      big.NewInt(2),
		QueuedDecrease:       big.NewInt(3),
		QueuedDecreaseWeight: big.NewInt(4),
	}

	got := base.Add(inc)
	assert.Same(t, base, got)
	assert.Equal(t, big.NewInt(101), got.ExitedTVL)
	assert.Equal(t, big.NewInt(202), got.ExitedTVLWeight)
	assert.Equal(t, big.NewInt(303), got.QueuedDecrease)
	assert.Equal(t, big.NewInt(404), got.QueuedDecreaseWeight)
}

func TestExit_Add_Nil(t *testing.T) {
	base := &Exit{
		ExitedTVL:            big.NewInt(10),
		ExitedTVLWeight:      big.NewInt(20),
		QueuedDecrease:       big.NewInt(30),
		QueuedDecreaseWeight: big.NewInt(40),
	}
	got := base.Add(nil)
	assert.Same(t, base, got)
	assert.Equal(t, big.NewInt(10), got.ExitedTVL)
	assert.Equal(t, big.NewInt(20), got.ExitedTVLWeight)
	assert.Equal(t, big.NewInt(30), got.QueuedDecrease)
	assert.Equal(t, big.NewInt(40), got.QueuedDecreaseWeight)
}
