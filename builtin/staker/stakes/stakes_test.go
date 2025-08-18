// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package stakes

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewWeightedStake_Basics(t *testing.T) {
	ws := NewWeightedStake(big.NewInt(1000), 50)
	assert.Equal(t, big.NewInt(1000), ws.VET())
	assert.Equal(t, big.NewInt(500), ws.Weight())
}

func TestNewWeightedStake_ZeroMultiplier(t *testing.T) {
	ws := NewWeightedStake(big.NewInt(1234), 0)
	assert.Equal(t, big.NewInt(1234), ws.VET())
	assert.Equal(t, big.NewInt(0), ws.Weight())
}

func TestNewWeightedStake_FullMultiplier(t *testing.T) {
	ws := NewWeightedStake(big.NewInt(999), 100)
	assert.Equal(t, big.NewInt(999), ws.VET())
	assert.Equal(t, big.NewInt(999), ws.Weight())
}

func TestNewWeightedStake_RoundingDown(t *testing.T) {
	ws := NewWeightedStake(big.NewInt(1001), 33)
	assert.Equal(t, big.NewInt(1001), ws.VET())
	assert.Equal(t, big.NewInt(330), ws.Weight())
}

func TestNewWeightedStake_LargeValues(t *testing.T) {
	vet := new(big.Int).Mul(big.NewInt(1_000_000_000), big.NewInt(1_000_000_000)) // 1e18
	ws := NewWeightedStake(vet, 25)
	want := new(big.Int).Div(new(big.Int).Mul(vet, big.NewInt(25)), big.NewInt(100))
	assert.Equal(t, vet, ws.VET())
	assert.Equal(t, want, ws.Weight())
}
