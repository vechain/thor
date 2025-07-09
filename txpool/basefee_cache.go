// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/thor"
)

// baseFeeCache caches base fee
type baseFeeCache struct {
	cache      *cache.PrioCache
	forkConfig *thor.ForkConfig
}

func newBaseFeeCache(forkConfig *thor.ForkConfig) *baseFeeCache {
	return &baseFeeCache{
		cache:      cache.NewPrioCache(32),
		forkConfig: forkConfig,
	}
}

// Get returns the base fee with the given parent block.
// Before GALACTICA, the base fee is not set, so it returns nil.
func (p *baseFeeCache) Get(parent *block.Header) *big.Int {
	if parent.Number()+1 < p.forkConfig.GALACTICA {
		return nil
	}

	if val, _, ok := p.cache.Get(parent.ID()); ok {
		return val.(*big.Int)
	}

	baseFee := galactica.CalcBaseFee(parent, p.forkConfig)
	p.cache.Set(parent.ID(), baseFee, float64(parent.Number()))
	return baseFee
}
