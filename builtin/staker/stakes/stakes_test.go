// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package stakes

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewWeightedStakeWithMultiplier_Basics(t *testing.T) {
	ws := NewWeightedStakeWithMultiplier(1000, 50)
	assert.Equal(t, uint64(1000), ws.VET)
	assert.Equal(t, uint64(500), ws.Weight)
}

func TestNewWeightedStakeWithMultiplier_ZeroMultiplier(t *testing.T) {
	ws := NewWeightedStakeWithMultiplier(1234, 0)
	assert.Equal(t, uint64(1234), ws.VET)
	assert.Equal(t, uint64(0), ws.Weight)
}

func TestNewWeightedStakeWithMultiplier_FullMultiplier(t *testing.T) {
	ws := NewWeightedStakeWithMultiplier(999, 100)
	assert.Equal(t, uint64(999), ws.VET)
	assert.Equal(t, uint64(999), ws.Weight)
}

func TestNewWeightedStakeWithMultiplier_RoundingDown(t *testing.T) {
	ws := NewWeightedStakeWithMultiplier(1001, 33)
	assert.Equal(t, uint64(1001), ws.VET)
	assert.Equal(t, uint64(330), ws.Weight)
}

func TestNewWeightedStakeWithMultiplier_LargeValues(t *testing.T) {
	vet := uint64(1e15) // 1e15
	ws := NewWeightedStakeWithMultiplier(vet, 25)
	want := uint64(1e15 * 25 / 100)
	assert.Equal(t, vet, ws.VET)
	assert.Equal(t, want, ws.Weight)
}

func TestOverFlow(t *testing.T) {
	a := NewWeightedStake(1, 1)
	b := NewWeightedStake(math.MaxUint64, 0)
	assert.ErrorContains(t, a.Add(b), "VET add overflow occurred")

	c := NewWeightedStake(0, math.MaxUint64)
	assert.ErrorContains(t, a.Add(c), "weight add overflow occurred")
}

func TestWeightedStake_Sub(t *testing.T) {
	a := NewWeightedStake(10, 20)
	b := NewWeightedStake(3, 5)
	// Normal subtraction
	assert.NoError(t, a.Sub(b))
	assert.Equal(t, uint64(7), a.VET)
	assert.Equal(t, uint64(15), a.Weight)

	// VET underflow
	a = NewWeightedStake(1, 1)
	b = NewWeightedStake(2, 0)
	err := a.Sub(b)
	assert.ErrorContains(t, err, "VET subtract underflow occurred")

	// Weight() underflow
	a = NewWeightedStake(2, 1)
	b = NewWeightedStake(1, 2)
	err = a.Sub(b)
	assert.ErrorContains(t, err, "weight subtract underflow occurred")
}

func TestWeightedStake_Clone(t *testing.T) {
	orig := NewWeightedStake(42, 99)
	clone := orig.Clone()
	assert.Equal(t, orig.VET, clone.VET)
	assert.Equal(t, orig.Weight, clone.Weight)
	// Ensure it's a different pointer
	assert.NotSame(t, orig, clone)
}
