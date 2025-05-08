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
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
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
	assert.False(t, cache.blockBaseFeeCache.Contains(postGalacticaHeader.ID()))
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

// Test that we read the legacy base gas price out of state correctly.
func TestGetLegacyTxBaseGasPrice_StorageRetrieval(t *testing.T) {
	// 1) build a fresh in-memory state and set the legacy price
	db := muxdb.NewMem()
	st1 := state.New(db, trie.Root{})
	wantPrice := big.NewInt(888)
	st1.SetStorage(builtin.Params.Address, thor.KeyLegacyTxBaseGasPrice, thor.BytesToBytes32(wantPrice.Bytes()))

	// 3) build a dummy header
	hdr := new(block.Builder).
		ParentID(createParentID(0)).
		Timestamp(1).
		TotalScore(1).
		GasLimit(1).
		GasUsed(0).
		Beneficiary(thor.Address{}).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Build().Header()

	// 4) call getLegacyTxBaseGasPrice
	cache := newGasPriceCache(&thor.ForkConfig{}, 2)
	got, err := cache.getLegacyTxBaseGasPrice(st1, hdr)
	assert.NoError(t, err)
	assert.Equal(t, wantPrice, got, "should read the exact stored price")
}

// Test that a second call for the same header hits the cache (no extra state reads).
func TestGetLegacyTxBaseGasPrice_CacheHit(t *testing.T) {
	db := muxdb.NewMem()
	st1 := state.New(db, trie.Root{})
	price := big.NewInt(777)
	st1.SetStorage(builtin.Params.Address, thor.KeyLegacyTxBaseGasPrice, thor.BytesToBytes32(price.Bytes()))

	hdr := new(block.Builder).
		ParentID(createParentID(1)).
		Timestamp(1).
		TotalScore(1).
		GasLimit(1).
		GasUsed(0).
		Beneficiary(thor.Address{}).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Build().Header()

	cache := newGasPriceCache(&thor.ForkConfig{}, 2)

	p1, err1 := cache.getLegacyTxBaseGasPrice(st1, hdr)
	assert.NoError(t, err1)

	// mutate underlying state (would change what Get would return if called)
	st1.SetStorage(builtin.Params.Address, thor.KeyLegacyTxBaseGasPrice, thor.BytesToBytes32(big.NewInt(1).Bytes()))

	p2, err2 := cache.getLegacyTxBaseGasPrice(st1, hdr)
	assert.NoError(t, err2)

	// both calls must return the original price—and identical *big.Int pointer
	assert.Equal(t, price, p2, "cache must hold the original value")
	assert.True(t, p1 == p2, "should return the same *big.Int instance on cache-hit")
}

// Test that once we exceed the cache limit entries get evicted.
func TestGetLegacyTxBaseGasPrice_CacheEviction(t *testing.T) {
	// limit = 1 so second insert evicts the first
	cache := newGasPriceCache(&thor.ForkConfig{}, 1)

	// set up a shared in-memory state with one price
	db := muxdb.NewMem()
	st1 := state.New(db, trie.Root{})
	base := big.NewInt(42)
	st1.SetStorage(builtin.Params.Address, thor.KeyLegacyTxBaseGasPrice, thor.BytesToBytes32(base.Bytes()))

	// helper to build header for a given block number
	buildHdr := func(n uint32) *block.Header {
		return new(block.Builder).
			ParentID(createParentID(n - 1)).
			Timestamp(1).
			TotalScore(1).
			GasLimit(1).
			GasUsed(0).
			Beneficiary(thor.Address{}).
			StateRoot(thor.Bytes32{}).
			ReceiptsRoot(thor.Bytes32{}).
			Build().Header()
	}

	hdr1 := buildHdr(10)
	hdr2 := buildHdr(20)

	// first insertion
	p1, err := cache.getLegacyTxBaseGasPrice(st1, hdr1)
	assert.NoError(t, err)
	assert.Equal(t, base, p1)

	// second insertion → should evict hdr1
	p2, err := cache.getLegacyTxBaseGasPrice(st1, hdr2)
	assert.NoError(t, err)
	assert.Equal(t, base, p2)

	assert.False(t, cache.legacyBaseFeeCache.Contains(hdr1.ID()),
		"first entry should be evicted when limit=1")
	assert.True(t, cache.legacyBaseFeeCache.Contains(hdr2.ID()),
		"second entry should remain in cache")
}
