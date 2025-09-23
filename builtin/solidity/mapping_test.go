// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

func newXenv() *xenv.Environment {
	contract := &vm.Contract{
		Gas: math.MaxUint64,
	}
	return xenv.New(nil, nil, nil, nil, nil, nil, contract, 0)
}

type TestStruct struct {
	Field1 uint64
	Field2 uint64
	Addr1  thor.Address
	Bytes1 thor.Bytes32
}

// BigStruct spans multiple slots: 3 Bytes32 fields.
type BigStruct struct {
	A thor.Bytes32
	B thor.Bytes32
	C thor.Bytes32
}

// newTestContext returns a fresh Context with in-memory DB and unlimited gas.
func newTestContext() *Context {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	charger := gascharger.New(newXenv())
	return &Context{address: thor.Address{1}, state: st, charger: charger}
}

// SetupMapping returns a new Mapping and its associated Charger.
func SetupMapping[V comparable]() (*gascharger.Charger, *Mapping[thor.Bytes32, V]) {
	ctx := newTestContext()
	mapping := NewMapping[thor.Bytes32, V](ctx, thor.Bytes32{1})
	return ctx.charger, mapping
}

// newRandomStruct generates a random TestStruct pointer.
func newRandomStruct() *TestStruct {
	return &TestStruct{
		Field1: 100,
		Field2: 200,
		Addr1:  datagen.RandAddress(),
		Bytes1: datagen.RandomHash(),
	}
}

// newBigStruct generates a random BigStruct pointer.
func newBigStruct() *BigStruct {
	return &BigStruct{
		A: datagen.RandomHash(),
		B: datagen.RandomHash(),
		C: datagen.RandomHash(),
	}
}

func TestMapping_SetGet_StructPointer(t *testing.T) {
	charger, mapping := SetupMapping[*TestStruct]()
	key := datagen.RandomHash()
	value := newRandomStruct()

	t.Run("set new value charges SstoreSetGas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, value))
		assert.Equal(t, 2*thor.SstoreSetGas, charger.TotalGas(), "wrong gas for new struct")
	})

	t.Run("get existing value charges SloadGas and returns value", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, value))

		// reset charger to measure only load
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		got, err := mapping.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, got)
		assert.Equal(t, 2*thor.SloadGas, charger.TotalGas(), "wrong gas for get struct")
	})

	t.Run("set zero pointer clears storage and returns nil", func(t *testing.T) {
		// reset charger
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger
		require.NoError(t, mapping.Update(key, nil))
		assert.Equal(t, uint64(0), charger.TotalGas(), "expected no gas for clearing slot")

		got, err := mapping.Get(key)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("get empty key charges no gas and returns nil", func(t *testing.T) {
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		got, err := mapping.Get(datagen.RandomHash())
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Equal(t, thor.SloadGas, charger.TotalGas(), "expected no gas for empty key")
	})

	t.Run("reset existing with newValue=false charges ResetGas", func(t *testing.T) {
		// pre-populate
		require.NoError(t, mapping.Insert(key, value))
		// reset charger
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		require.NoError(t, mapping.Update(key, newRandomStruct()))
		assert.Equal(t, 2*thor.SstoreResetGas, charger.TotalGas(), "wrong gas for reset struct")
	})

	t.Run("overwrite existing with newValue=true charges SetGas", func(t *testing.T) {
		// pre-populate
		require.NoError(t, mapping.Insert(key, value))
		// reset charger
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		require.NoError(t, mapping.Insert(key, newRandomStruct()))
		assert.Equal(t, 2*thor.SstoreSetGas, charger.TotalGas(), "wrong gas for overwrite struct")
	})
}

func TestMapping_SetGet_AddressValue(t *testing.T) {
	charger, mapping := SetupMapping[thor.Address]()
	key := datagen.RandomHash()
	addr := datagen.RandAddress()

	t.Run("set non-zero address charges gas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, addr))
		assert.Equal(t, thor.SstoreSetGas, charger.TotalGas(), "wrong gas for new address")
	})

	t.Run("get address returns default zero when none set and no gas", func(t *testing.T) {
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		got, err := mapping.Get(datagen.RandomHash())
		require.NoError(t, err)
		assert.Equal(t, thor.Address{}, got)
		assert.Equal(t, thor.SloadGas, charger.TotalGas(), "expected no gas for empty address key")
	})

	t.Run("clear address by setting zero-value and no gas", func(t *testing.T) {
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger
		require.NoError(t, mapping.Update(key, thor.Address{}))
		assert.Equal(t, uint64(0), charger.TotalGas())
	})
}

func TestMapping_SetGet_AddressPointer(t *testing.T) {
	charger, mapping := SetupMapping[*thor.Address]()
	key := datagen.RandomHash()
	addr := datagen.RandAddress()

	t.Run("set non-nil pointer charges gas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, &addr))
		assert.Equal(t, thor.SstoreSetGas, charger.TotalGas(), "wrong gas for pointer address set")
	})

	t.Run("get pointer returns nil when not set and no gas", func(t *testing.T) {
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		got, err := mapping.Get(datagen.RandomHash())
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Equal(t, thor.SloadGas, charger.TotalGas(), "expected no gas for empty pointer key")
	})

	t.Run("clear pointer by setting nil and no gas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, nil))
		assert.Equal(t, thor.SloadGas, charger.TotalGas(), "expected no gas for clearing pointer slot")
		got, err := mapping.Get(key)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestMapping_SetGet_Uint64Value(t *testing.T) {
	charger, mapping := SetupMapping[uint64]()
	key := datagen.RandomHash()
	val := uint64(42)

	t.Run("set non-zero uint64 charges gas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, val))
		assert.Equal(t, thor.SstoreSetGas, charger.TotalGas(), "wrong gas for uint64 set")
	})

	t.Run("get default zero returns zero and no gas", func(t *testing.T) {
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger
		got, err := mapping.Get(datagen.RandomHash())
		require.NoError(t, err)
		assert.Equal(t, uint64(0), got)
		assert.Equal(t, thor.SloadGas, charger.TotalGas(), "expected no gas for empty uint64 key")
	})

	t.Run("clear uint64 by setting zero-value and no gas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, 0))
		assert.Equal(t, thor.SloadGas, charger.TotalGas(), "expected no gas for clearing uint64 slot")
	})
}

func TestMapping_MultiSlotValue(t *testing.T) {
	// BigStruct spans ~100 bytes RLP → 4 slots
	charger, mapping := SetupMapping[*BigStruct]()
	key := datagen.RandomHash()
	value := newBigStruct()

	t.Run("set big struct charges correct SstoreSetGas", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, value))
		// maximum 2 slots, defined by gas.go
		expected := 2 * thor.SstoreSetGas
		assert.Equal(t, expected, charger.TotalGas(), "wrong gas for big struct set")
	})

	t.Run("get big struct charges correct SloadGas and returns value", func(t *testing.T) {
		require.NoError(t, mapping.Insert(key, value))
		charger = gascharger.New(newXenv())
		mapping.context.charger = charger

		got, err := mapping.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, got)
		// maximum 2 slots, defined by gas.go
		assert.Equal(t, 2*thor.SloadGas, charger.TotalGas(), "wrong gas for big struct get")
	})
}

func TestMappingGetSet_ErrorReturnsZeroAndErr(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	contract := thor.BytesToAddress([]byte("map"))
	ctx := NewContext(contract, st, nil)

	basePos := thor.BytesToBytes32([]byte("base"))
	m := NewMapping[thor.Address, thor.Address](ctx, basePos)

	key := thor.BytesToAddress([]byte("k"))
	slot := thor.Blake2b(key.Bytes(), basePos.Bytes())

	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})

	val, err := m.Get(key)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if val != (thor.Address{}) {
		t.Fatalf("expected zero value, got %v", val)
	}

	m2 := NewMapping[thor.Address, chan int](ctx, basePos)
	value := make(chan int)

	err = m2.Insert(key, value)
	if err == nil {
		t.Fatalf("expected encode error, got nil")
	}
}
