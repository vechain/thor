package txpool

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/thor"
)

type entry struct {
	baseFee *big.Int
}

type gasPriceCache struct {
	cache      *cache.PrioCache
	forkConfig *thor.ForkConfig
}

func newGasPriceCache(forkConfig *thor.ForkConfig, limit int) *gasPriceCache {
	return &gasPriceCache{
		cache:      cache.NewPrioCache(limit),
		forkConfig: forkConfig,
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
	if val, _, ok := c.cache.Get(blkHeader.ID()); ok {
		ent = val.(*entry)
		if ent.baseFee != nil {
			return ent.baseFee
		}
	} else {
		ent = &entry{}
	}

	ent.baseFee = fork.CalcBaseFee(c.forkConfig, blkHeader)

	c.cache.Set(blkHeader.ID(), ent, float64(blkHeader.Number()))
	return ent.baseFee
}
