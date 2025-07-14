// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

type TestStruct struct {
	Field1 uint64
	Field2 uint64
	Addr1  thor.Address
	Bytes1 thor.Bytes32
}

func newContext() *Context {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.Address{1}
	charger := gascharger.New(nil)

	return &Context{
		address: addr,
		state:   st,
		charger: charger,
	}
}

func newSetup() (*gascharger.Charger, *Mapping[thor.Bytes32, *TestStruct]) {
	context := newContext()
	mapping := NewMapping[thor.Bytes32, *TestStruct](context, thor.Bytes32{1})

	return context.charger, mapping
}

func newTestStruct() *TestStruct {
	return &TestStruct{
		Field1: 100,
		Field2: 200,
		Addr1:  datagen.RandAddress(),
		Bytes1: datagen.RandomHash(),
	}
}

func TestMapping_Set_NewValue(t *testing.T) {
	charger, mapping := newSetup()
	assert.NoError(t, mapping.Set(datagen.RandomHash(), newTestStruct(), true))
	assert.Equalf(t, charger.TotalGas(), 2*thor.SstoreSetGas, "Expected  SSTORE operations, got %s", charger.Breakdown())
}

func TestMapping_Set_ExistingValue(t *testing.T) {
	charger, mapping := newSetup()
	assert.NoError(t, mapping.Set(datagen.RandomHash(), newTestStruct(), false))
	assert.Equalf(t, charger.TotalGas(), 2*thor.SstoreResetGas, "Expected  SSTORE operations, got %s", charger.Breakdown())
}

func TestMapping_Get_ExistingValue(t *testing.T) {
	_, mapping := newSetup()
	key := datagen.RandomHash()
	value := newTestStruct()
	assert.NoError(t, mapping.Set(key, value, true))

	charger := gascharger.New(nil)    // Reset charger to measure only the SLOAD operation
	mapping.context.charger = charger // Reset charger to measure only the SLOAD operation

	retrievedValue, err := mapping.Get(key)
	assert.NoError(t, err)
	assert.Equal(t, value, retrievedValue)
	assert.Equalf(t, charger.TotalGas(), 2*thor.SloadGas, "Expected  SLOAD operation, got %s", charger.Breakdown())
}
