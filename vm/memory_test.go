// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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

func TestMemoryCopyPanic(t *testing.T) {
	mem := NewMemory()
	mem.Resize(10)

	assert.Panics(t, func() {
		mem.Copy(0, 0, uint64(mem.Len()+1))
	}, "Copy should panic when length exceeds store size")

	assert.Panics(t, func() {
		mem.Copy(5, 0, 8)
	}, "Copy should panic when dst + length exceeds store size")

	assert.Panics(t, func() {
		mem.Copy(0, 5, 8)
	}, "Copy should panic when src + length exceeds store size")
}

func TestMemoryCopy(t *testing.T) {
	// Test cases from https://eips.ethereum.org/EIPS/eip-5656#test-cases
	for i, tc := range []struct {
		dst, src, len uint64
		pre           string
		want          string
	}{
		{ // MCOPY 0 32 32 - copy 32 bytes from offset 32 to offset 0.
			0, 32, 32,
			"0000000000000000000000000000000000000000000000000000000000000000 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
			"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		},

		{ // MCOPY 0 0 32 - copy 32 bytes from offset 0 to offset 0.
			0, 0, 32,
			"0101010101010101010101010101010101010101010101010101010101010101",
			"0101010101010101010101010101010101010101010101010101010101010101",
		},
		{ // MCOPY 0 1 8 - copy 8 bytes from offset 1 to offset 0 (overlapping).
			0, 1, 8,
			"000102030405060708 000000000000000000000000000000000000000000000000",
			"010203040506070808 000000000000000000000000000000000000000000000000",
		},
		{ // MCOPY 1 0 8 - copy 8 bytes from offset 0 to offset 1 (overlapping).
			1, 0, 8,
			"000102030405060708 000000000000000000000000000000000000000000000000",
			"000001020304050607 000000000000000000000000000000000000000000000000",
		},
		{ // MCOPY 0xFFFFFFFFFFFF 0xFFFFFFFFFFFF 0 - copy zero bytes from out-of-bounds index(overlapping).
			0xFFFFFFFFFFFF, 0xFFFFFFFFFFFF, 0,
			"11",
			"11",
		},
		{ // MCOPY 0xFFFFFFFFFFFF 0 0 - copy zero bytes from start of mem to out-of-bounds.
			0xFFFFFFFFFFFF, 0, 0,
			"11",
			"11",
		},
		{ // MCOPY 0 0xFFFFFFFFFFFF 0 - copy zero bytes from out-of-bounds to start of mem
			0, 0xFFFFFFFFFFFF, 0,
			"11",
			"11",
		},
	} {
		m := NewMemory()
		// Clean spaces
		data := common.FromHex(strings.ReplaceAll(tc.pre, " ", ""))
		// Set pre
		m.Resize(uint64(len(data)))
		m.Set(0, uint64(len(data)), data)
		// Do the copy
		m.Copy(tc.dst, tc.src, tc.len)
		want := common.FromHex(strings.ReplaceAll(tc.want, " ", ""))
		if have := m.store; !bytes.Equal(want, have) {
			t.Errorf("case %d: want: %#x\nhave: %#x\n", i, want, have)
		}
	}
}
