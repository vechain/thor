// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/thor"
)

func TestUint256(t *testing.T) {
	ctx := newContext()
	uint := NewUint256(ctx, thor.Bytes32{01})

	// test `Set`
	uint.Set(big.NewInt(1000))
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas)

	// test `Get`
	charger := gascharger.New(nil)
	ctx.charger = charger

	value, err := uint.Get()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), value)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SloadGas)

	// test `Add`
	charger = gascharger.New(nil)
	ctx.charger = charger

	err = uint.Add(big.NewInt(500))
	assert.NoError(t, err)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas+thor.SloadGas)

	value, err = uint.Get()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1500), value)

	// test `Sub`
	charger = gascharger.New(nil)
	ctx.charger = charger

	err = uint.Sub(big.NewInt(200))
	assert.NoError(t, err)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas+thor.SloadGas)

	value, err = uint.Get()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1300), value)
}
