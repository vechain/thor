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

func TestAddress(t *testing.T) {
	ctx := newContext()
	address := NewAddress(ctx, thor.Bytes32{1})

	value := datagen.RandAddress()

	// Test `Set` new value
	address.Set(&value, true)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreSetGas)
	charger := gascharger.New(newXenv())
	ctx.charger = charger

	// Test `Set` updated value
	address.Set(&value, false)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas)
	charger = gascharger.New(newXenv())
	ctx.charger = charger

	// Test `Get`
	retrievedValue, err := address.Get()
	assert.NoError(t, err)
	assert.Equal(t, value, retrievedValue)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SloadGas)
}
