// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package stakes

import (
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
	vet := uint64(1e15) //1e10
	ws := NewWeightedStakeWithMultiplier(vet, 25)
	want := uint64(1e15 * 25 / 100)
	assert.Equal(t, vet, ws.VET)
	assert.Equal(t, want, ws.Weight)
}
