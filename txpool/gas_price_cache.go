// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type entry struct {
	baseFee *big.Int
}

type gasPriceCache struct {
	blockBaseFeeCache  *cache.PrioCache
	legacyBaseFeeCache *cache.PrioCache
	forkConfig         *thor.ForkConfig
}

func newGasPriceCache(forkConfig *thor.ForkConfig, limit int) *gasPriceCache {
	return &gasPriceCache{
		blockBaseFeeCache:  cache.NewPrioCache(limit),
		legacyBaseFeeCache: cache.NewPrioCache(limit),
		forkConfig:         forkConfig,
	}
}

// getBlockBaseFee returns the base fee for the given block
// avoids recomputing every time, and caches the result
func (c *gasPriceCache) getBlockBaseFee(blkHeader *block.Header) *big.Int {
	if blkHeader.Number()+1 < c.forkConfig.GALACTICA {
		// Before GALACTICA, the base fee is not set, so it returns nil.
		return nil
	}

	var ent *entry
	if val, _, ok := c.blockBaseFeeCache.Get(blkHeader.ID()); ok {
		ent = val.(*entry)
		if ent.baseFee != nil {
			return ent.baseFee
		}
	} else {
		ent = &entry{}
	}

	ent.baseFee = fork.CalcBaseFee(c.forkConfig, blkHeader)

	c.blockBaseFeeCache.Set(blkHeader.ID(), ent, float64(blkHeader.Number()))
	return ent.baseFee
}

// GetLegacyTxBaseGasPrice returns the legacy tx base gas price for the given block.
func (c *gasPriceCache) getLegacyTxBaseGasPrice(state *state.State, head *block.Header) (*big.Int, error) {
	var ent *entry
	if val, _, ok := c.legacyBaseFeeCache.Get(head.ID()); ok {
		ent = val.(*entry)
		if ent.baseFee != nil {
			return ent.baseFee, nil
		}
	} else {
		ent = &entry{}
	}

	baseGasPrice, err := builtin.Params.Native(state).Get(thor.KeyLegacyTxBaseGasPrice)
	if err != nil {
		return nil, err
	}
	ent.baseFee = baseGasPrice

	c.legacyBaseFeeCache.Set(head.ID(), ent, float64(head.Number()))
	return ent.baseFee, nil
}
