// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/thor"
)

func TestRaw_NewRaw(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-position"))

	raw := NewRaw[uint64](ctx, pos)
	assert.NotNil(t, raw)
	assert.Equal(t, ctx, raw.context)
	assert.Equal(t, pos, raw.pos)
}

func TestRaw_Get_EmptyStorage(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-get-empty"))
	raw := NewRaw[uint64](ctx, pos)

	// Test getting from empty storage
	value, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), value)
	assert.Equal(t, thor.SloadGas, ctx.charger.TotalGas())
}

func TestRaw_Get_ExistingValue(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-get-existing"))
	raw := NewRaw[uint64](ctx, pos)

	// Set a value first
	expectedValue := uint64(12345)
	err := raw.Insert(expectedValue)
	require.NoError(t, err)

	// Reset charger to measure only get operation
	ctx.charger = gascharger.New(newXenv())

	// Test getting existing value
	value, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, expectedValue, value)
	assert.Equal(t, thor.SloadGas, ctx.charger.TotalGas())
}

func TestRaw_Insert_NewValue(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-insert"))
	raw := NewRaw[uint64](ctx, pos)

	// Test inserting new value
	value := uint64(54321)
	err := raw.Insert(value)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreSetGas, ctx.charger.TotalGas())

	// Verify the value was stored
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}

func TestRaw_Update_ExistingValue(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-update"))
	raw := NewRaw[uint64](ctx, pos)

	// Insert initial value
	initialValue := uint64(100)
	err := raw.Insert(initialValue)
	require.NoError(t, err)

	// Reset charger
	ctx.charger = gascharger.New(newXenv())

	// Test updating existing value
	newValue := uint64(200)
	err = raw.Update(newValue)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreResetGas, ctx.charger.TotalGas())

	// Verify the value was updated
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, newValue, retrieved)
}

func TestRaw_Upsert_NewValue(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-upsert-new"))
	raw := NewRaw[uint64](ctx, pos)

	// Test upserting new value (should use Insert)
	value := uint64(999)
	err := raw.Upsert(value)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreSetGas, ctx.charger.TotalGas())

	// Verify the value was stored
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}

func TestRaw_Upsert_ExistingValue(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-upsert-existing"))
	raw := NewRaw[uint64](ctx, pos)

	// Insert initial value
	initialValue := uint64(500)
	err := raw.Insert(initialValue)
	require.NoError(t, err)

	// Reset charger
	ctx.charger = gascharger.New(newXenv())

	// Test upserting existing value (should use Update)
	newValue := uint64(600)
	err = raw.Upsert(newValue)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreResetGas, ctx.charger.TotalGas())

	// Verify the value was updated
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, newValue, retrieved)
}

func TestRaw_ZeroValue(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-zero-value"))
	raw := NewRaw[uint64](ctx, pos)

	// Insert a value first
	err := raw.Insert(uint64(100))
	require.NoError(t, err)

	// Reset charger
	ctx.charger = gascharger.New(newXenv())

	// Test setting zero value (should clear storage)
	err = raw.Update(uint64(0))
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreResetGas, ctx.charger.TotalGas())

	// Verify storage was cleared
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), retrieved)
}

func TestRaw_ComplexType(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-complex"))

	type TestStruct struct {
		Field1 uint64
		Field2 string
		Field3 thor.Address
	}

	raw := NewRaw[TestStruct](ctx, pos)

	// Test with complex struct
	value := TestStruct{
		Field1: 12345,
		Field2: "test-string",
		Field3: thor.Address{1, 2, 3, 4, 5},
	}

	err := raw.Insert(value)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreSetGas, ctx.charger.TotalGas())

	// Verify the struct was stored correctly
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}

func TestRaw_StringType(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-string"))
	raw := NewRaw[string](ctx, pos)

	// Test with string
	value := "hello world"
	err := raw.Insert(value)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreSetGas, ctx.charger.TotalGas())

	// Verify the string was stored correctly
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}

func TestRaw_AddressType(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-address"))
	raw := NewRaw[thor.Address](ctx, pos)

	// Test with address
	value := thor.Address{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
	err := raw.Insert(value)
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreSetGas, ctx.charger.TotalGas())

	// Verify the address was stored correctly
	ctx.charger = gascharger.New(newXenv())
	retrieved, err := raw.Get()
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)
}

func TestRaw_GasCharging(t *testing.T) {
	ctx := newTestContext()
	pos := thor.BytesToBytes32([]byte("test-gas"))
	raw := NewRaw[uint64](ctx, pos)

	// Test gas charging for different operations
	initialGas := ctx.charger.TotalGas()

	// Insert should charge SstoreSetGas
	err := raw.Insert(uint64(100))
	require.NoError(t, err)
	assert.Equal(t, initialGas+thor.SstoreSetGas, ctx.charger.TotalGas())

	// Get should charge SloadGas
	ctx.charger = gascharger.New(newXenv())
	_, err = raw.Get()
	require.NoError(t, err)
	assert.Equal(t, thor.SloadGas, ctx.charger.TotalGas())

	// Update should charge SstoreResetGas
	ctx.charger = gascharger.New(newXenv())
	err = raw.Update(uint64(200))
	require.NoError(t, err)
	assert.Equal(t, thor.SstoreResetGas, ctx.charger.TotalGas())
}
