// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/genesis"
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

	tchain.MintBlock(genesis.DevAccounts()[0])

	// GALACTICA as next block
	baseFee = cache.Get(repo.BestBlockSummary().Header)
	assert.NotNil(t, baseFee)
	assert.Equal(t, big.NewInt(thor.InitialBaseFee), baseFee)

	val, _, ok := cache.cache.Get(repo.BestBlockSummary().Header.ID())
	assert.True(t, ok)
	assert.Equal(t, big.NewInt(thor.InitialBaseFee), val.(*big.Int))
}
