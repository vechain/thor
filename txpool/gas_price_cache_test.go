// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package txpool

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

func createParentID(blockNum uint32) thor.Bytes32 {
	var id thor.Bytes32
	binary.BigEndian.PutUint32(id[:], blockNum)
	return id
}

func TestGasPriceCache(t *testing.T) {
	// Create test fork config
	forkConfig := &thor.ForkConfig{
		GALACTICA: 1000, // Set GALACTICA fork at block 1000
	}

	// Create test cache with small limit
	cache := newGasPriceCache(forkConfig, 2)

	// Test pre-GALACTICA block
	preGalacticaHeader := new(block.Builder).
		ParentID(createParentID(997)). // Parent of block 998
		Timestamp(1000).
		TotalScore(100).
		GasLimit(10000000).
		GasUsed(0).
		Beneficiary(thor.Address{}).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Build().Header()

	baseFee := cache.getBlockBaseFee(preGalacticaHeader)
	assert.Nil(t, baseFee, "Base fee should be nil for pre-GALACTICA blocks")

	// Test post-GALACTICA block
	postGalacticaHeader := new(block.Builder).
		ParentID(createParentID(999)). // Parent of block 1000
		Timestamp(1000).
		TotalScore(100).
		GasLimit(10000000).
		GasUsed(0).
		Beneficiary(thor.Address{}).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		BaseFee(big.NewInt(1000)).
		Build().Header()

	baseFee = cache.getBlockBaseFee(postGalacticaHeader)
	assert.NotNil(t, baseFee, "Base fee should not be nil for post-GALACTICA blocks")

	// Test cache hit
	baseFee2 := cache.getBlockBaseFee(postGalacticaHeader)
	assert.Equal(t, baseFee, baseFee2, "Cached base fee should match")

	// Test cache recalculation
	// Add more blocks to force eviction
	header3 := new(block.Builder).
		ParentID(createParentID(1000)). // Parent of block 1001
		Timestamp(1000).
		TotalScore(100).
		GasLimit(10000000).
		GasUsed(40_000_000).
		Beneficiary(thor.Address{}).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		BaseFee(big.NewInt(1001)).
		Build().Header()

	header4 := new(block.Builder).
		ParentID(createParentID(1001)). // Parent of block 1002
		Timestamp(1000).
		TotalScore(100).
		GasLimit(10000000).
		GasUsed(40_000_000).
		Beneficiary(thor.Address{}).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		BaseFee(big.NewInt(1002)).
		Build().Header()

	cache.getBlockBaseFee(header3)
	cache.getBlockBaseFee(header4)

	// First block should be evicted and values recalculated
	assert.False(t, cache.cache.Contains(postGalacticaHeader.ID()))
}

func TestGasPriceCacheWithMockFork(t *testing.T) {
	// Create a mock fork config
	forkConfig := &thor.ForkConfig{
		GALACTICA: 1000,
	}

	// Create test cache
	cache := newGasPriceCache(forkConfig, 10)

	// Test multiple blocks with different numbers
	testCases := []struct {
		blockNumber uint32
		shouldBeNil bool
	}{
		{997, true},   // Pre-GALACTICA
		{1000, false}, // At GALACTICA
		{1001, false}, // Post-GALACTICA
		{2000, false}, // Far post-GALACTICA
	}

	for _, tc := range testCases {
		header := new(block.Builder).
			ParentID(createParentID(tc.blockNumber - 1)). // Parent of current block
			Timestamp(1000).
			TotalScore(100).
			GasLimit(10000000).
			GasUsed(0).
			Beneficiary(thor.Address{}).
			StateRoot(thor.Bytes32{}).
			ReceiptsRoot(thor.Bytes32{}).
			BaseFee(big.NewInt(1000)).
			Build().Header()

		baseFee := cache.getBlockBaseFee(header)
		if tc.shouldBeNil {
			assert.Nil(t, baseFee, "Base fee should be nil for block %d", tc.blockNumber)
		} else {
			assert.NotNil(t, baseFee, "Base fee should not be nil for block %d", tc.blockNumber)
		}
	}
}

func TestGasPriceCacheConcurrentAccess(t *testing.T) {
	forkConfig := &thor.ForkConfig{
		GALACTICA: 1000,
	}
	cache := newGasPriceCache(forkConfig, 100)

	// Create a channel to signal when all goroutines are done
	done := make(chan bool)

	// Launch multiple goroutines to test concurrent access
	for i := range [10]struct{}{} {
		go func(blockNum uint32) {
			header := new(block.Builder).
				ParentID(createParentID(blockNum - 1)). // Parent of current block
				Timestamp(1000).
				TotalScore(100).
				GasLimit(10000000).
				GasUsed(0).
				Beneficiary(thor.Address{}).
				StateRoot(thor.Bytes32{}).
				ReceiptsRoot(thor.Bytes32{}).
				BaseFee(big.NewInt(1000)).
				Build().Header()

			cache.getBlockBaseFee(header)
			done <- true
		}(uint32(1000 + i))
	}

	// Wait for all goroutines to complete
	for range [10]struct{}{} {
		<-done
	}
}
