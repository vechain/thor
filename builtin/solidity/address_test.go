// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestAddress(t *testing.T) {
	ctx := newContext()
	state := ctx.state
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

	address.Set(nil, true)
	retrievedValue, err = address.Get()
	assert.NoError(t, err)
	assert.Equal(t, thor.Address{}, retrievedValue)

	ctxAddr := ctx.Address()
	assert.Equal(t, thor.Address{1}, ctxAddr)

	ctxState := ctx.State()
	assert.Equal(t, state, ctxState)
}

func TestAddress_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	contract := thor.BytesToAddress([]byte("addr"))
	slot := thor.BytesToBytes32([]byte("slot"))

	// Write invalid RLP to storage so state.GetStorage fails
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})

	ctx := NewContext(contract, st, nil)
	a := NewAddress(ctx, slot)

	addr, err := a.Get()
	assert.Equal(t, thor.Address{}, addr)
	assert.Error(t, err, "")
}
