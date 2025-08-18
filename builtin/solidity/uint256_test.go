// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func newContext() *Context {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.Address{1}
	charger := gascharger.New(newXenv())

	return &Context{
		address: addr,
		state:   st,
		charger: charger,
	}
}

func TestUint256(t *testing.T) {
	ctx := newContext()
	uint := NewUint256(ctx, thor.BytesToBytes32([]byte("test-uint256")))

	// test `Set`
	uint.Set(big.NewInt(1000))
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas)

	// test `Get`
	charger := gascharger.New(newXenv())
	ctx.charger = charger

	value, err := uint.Get()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), value)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SloadGas)

	// test `Add`
	charger = gascharger.New(newXenv())
	ctx.charger = charger

	err = uint.Add(big.NewInt(500))
	assert.NoError(t, err)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas+thor.SloadGas)

	value, err = uint.Get()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1500), value)

	// test `Sub`
	charger = gascharger.New(newXenv())
	ctx.charger = charger

	err = uint.Sub(big.NewInt(200))
	assert.NoError(t, err)
	assert.Equal(t, ctx.charger.TotalGas(), thor.SstoreResetGas+thor.SloadGas)

	value, err = uint.Get()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1300), value)

	// test negative value
	err = uint.Set(big.NewInt(-100))
	assert.ErrorContains(t, err, "test-uint256 uint256 cannot be negative")

	// test overflow
	val := new(big.Int).Lsh(big.NewInt(1), 256)
	err = uint.Set(val)
	if assert.Error(t, err) {
		assert.Equal(t, "uint256 overflow: value exceeds 256 bits", err.Error())
	}

	// add 0
	err = uint.Add(big.NewInt(0))
	assert.NoError(t, err)

	// sub 0
	err = uint.Sub(big.NewInt(0))
	assert.NoError(t, err)

	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("cfg"))
	ctx = NewContext(addr, st, nil)

	uint = NewUint256(ctx, thor.BytesToBytes32([]byte("test-uint256")))
	st.SetRawStorage(addr, thor.BytesToBytes32([]byte("test-uint256")), rlp.RawValue{0xFF})
	value, err = uint.Get()
	assert.Nil(t, value)
	assert.Error(t, err, "")

	err = uint.Add(big.NewInt(1))
	assert.Nil(t, value)
	assert.Error(t, err, "")

	err = uint.Sub(big.NewInt(1))
	assert.Nil(t, value)
	assert.Error(t, err, "")
}
