// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

func TestBytes32(t *testing.T) {
	ctx := newContext()
	bytes32 := NewBytes32(ctx, thor.Bytes32{1})

	value := datagen.RandomHash()

	// Test `Set` new value
	bytes32.Set(&value, true)
	assert.Equal(t, ctx.Charger.TotalGas(), thor.SstoreSetGas)
	charger := gascharger.New(nil)
	ctx.Charger = charger

	// Test `Set` updated value
	bytes32.Set(&value, false)
	assert.Equal(t, ctx.Charger.TotalGas(), thor.SstoreResetGas)
	charger = gascharger.New(nil)
	ctx.Charger = charger

	// Test `Get`
	retrievedValue, err := bytes32.Get()
	assert.NoError(t, err)
	assert.Equal(t, value, retrievedValue)
	assert.Equal(t, ctx.Charger.TotalGas(), thor.SloadGas)
}
