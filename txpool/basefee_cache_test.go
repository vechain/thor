// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

func TestCacheBaseFee(t *testing.T) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 2

	tchain, err := testchain.NewWithFork(&forkConfig, 180)
	assert.NoError(t, err)

	repo := tchain.Repo()

	cache := newBaseFeeCache(&forkConfig)

	// before GALACTICA
	baseFee := cache.Get(repo.BestBlockSummary().Header)
	assert.Nil(t, baseFee)

	tchain.MintBlock()

	// GALACTICA as next block
	baseFee = cache.Get(repo.BestBlockSummary().Header)
	assert.NotNil(t, baseFee)
	assert.Equal(t, big.NewInt(thor.InitialBaseFee), baseFee)

	val, _, ok := cache.cache.Get(repo.BestBlockSummary().Header.ID())
	assert.True(t, ok)
	assert.Equal(t, big.NewInt(thor.InitialBaseFee), val.(*big.Int))
}

func TestBaseFeeCacheEvictsOldestAtCapacity(t *testing.T) {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 0
	cache := newBaseFeeCache(&forkConfig)

	parentID := thor.Bytes32{}
	var firstID, newestID thor.Bytes32
	for i := range 33 {
		header := new(block.Builder).
			ParentID(parentID).
			GasLimit(40_000_000).
			BaseFee(big.NewInt(thor.InitialBaseFee + int64(i))).
			Build().
			Header()
		require.NotNil(t, cache.Get(header))
		if i == 0 {
			firstID = header.ID()
		}
		newestID = header.ID()
		parentID = header.ID()
	}

	_, _, firstPresent := cache.cache.Get(firstID)
	_, _, newestPresent := cache.cache.Get(newestID)
	assert.False(t, firstPresent)
	assert.True(t, newestPresent)
	assert.Equal(t, 32, cache.cache.Len())
}
