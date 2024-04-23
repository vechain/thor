// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestNewMemory(t *testing.T) {
	mem := NewMemory()
	assert.NotNil(t, mem)
	mem.Set(1, 0, []byte{})

	mem.Print()
}

func TestSet(t *testing.T) {
	mem := NewMemory()
	assert.NotNil(t, mem)

	testData := []byte{1, 2, 3, 4}
	mem.Resize(uint64(len(testData)))
	mem.Set(0, uint64(len(testData)), testData)

	retrievedData := mem.GetPtr(0, int64(len(testData)))
	assert.Equal(t, testData, retrievedData)

	mem.Print()

	data := mem.Data()

	assert.NotNil(t, data)
}

func TestSet32AndGetCopy(t *testing.T) {
	mem := NewMemory()
	assert.NotNil(t, mem)

	// Create a uint256 value and set it in memory
	testValue := uint256.NewInt(123).SetUint64(12345678)
	mem.Resize(32) // Ensure there's enough space
	mem.Set32(0, testValue)

	// Retrieve the value using GetCopy
	copiedData := mem.GetCopy(0, 32)
	assert.NotNil(t, copiedData, "Copied data should not be nil")

	// Convert copiedData back to uint256 for comparison
	retrievedValue := new(uint256.Int).SetBytes(copiedData)
	assert.Equal(t, testValue, retrievedValue, "Retrieved value should match the original")
}

func TestSetPanic(t *testing.T) {
	mem := NewMemory()
	assert.NotNil(t, mem)

	// This should cause a panic because the memory store is not resized
	assert.Panics(t, func() {
		mem.Set(0, 10, []byte{1, 2, 3, 4}) // Trying to set 10 bytes in an empty store
	}, "Set should panic when trying to set more data than the size of the store")
}

func TestSet32Panic(t *testing.T) {
	mem := NewMemory()
	assert.NotNil(t, mem)

	// This should cause a panic because the memory store is not resized
	testVal := uint256.NewInt(123).SetUint64(12345)
	assert.Panics(t, func() {
		mem.Set32(0, testVal) // Trying to set 32 bytes in an empty store
	}, "Set32 should panic when trying to set 32 bytes in an empty store")
}
