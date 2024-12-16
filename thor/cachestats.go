// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package thor

import "sync/atomic"

// CacheStats is a utility for collecting cache hit/miss.
type CacheStats struct {
	hit, miss atomic.Int64
	flag      atomic.Int32
}

// Hit records a hit.
func (cs *CacheStats) Hit() int64 { return cs.hit.Add(1) }

// Miss records a miss.
func (cs *CacheStats) Miss() int64 { return cs.miss.Add(1) }

// Stats returns the number of hits and misses and whether
// the hit rate was changed comparing to the last call.
func (cs *CacheStats) Stats() (bool, int64, int64) {
	hit := cs.hit.Load()
	miss := cs.miss.Load()
	lookups := hit + miss

	hitRate := float64(0)
	if lookups > 0 {
		hitRate = float64(hit) / float64(lookups)
	}
	flag := int32(hitRate * 1000)

	return cs.flag.Swap(flag) != flag, hit, miss
}
