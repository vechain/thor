// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	Cache "github.com/vechain/thor/cache"
)

type priorCache struct {
	cache *Cache.PrioCache
}

func newPriorCache(limit int) *priorCache {
	return &priorCache{
		Cache.NewPrioCache(limit),
	}
}

func (pc *priorCache) Set(key, value interface{}) {
	if obj, ok := value.(*txObject); ok {
		pc.cache.Set(obj.tx.ID(), obj, float64(obj.overallGP.Uint64()))
	}
}

func (pc *priorCache) Get(key interface{}) (interface{}, bool) {
	value, _, ok := pc.cache.Get(key)
	return value, ok
}

func (pc *priorCache) Remove(key interface{}) bool {
	return pc.cache.Remove(key) != nil
}

func (pc *priorCache) Len() int {
	return pc.cache.Len()
}

func (pc *priorCache) ForEach(cb func(*Cache.Entry) bool) bool {
	forEach := func(pe *Cache.PrioEntry) bool {
		return cb(&pe.Entry)
	}
	return pc.cache.ForEach(forEach)
}
