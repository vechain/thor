// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package cache_test

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/cache"
)

func TestPrioCacheAddRemove(t *testing.T) {
	c := cache.NewPrioCache(16)
	c.Set("key", "value", 100)
	assert.True(t, c.Contains("key"))
	assert.Equal(t, 1, c.Len())

	v, p, b := c.Get("key")
	assert.Equal(t, "value", v)
	assert.Equal(t, float64(100), p)
	assert.Equal(t, true, b)

	assert.Equal(t, &cache.PrioEntry{Entry: cache.Entry{Key: "key", Value: "value"}, Priority: float64(100)}, c.Remove("key"))
	assert.Equal(t, 0, c.Len())

	_, _, b = c.Get("key")
	assert.Equal(t, false, b)
}

func TestPrioCache(t *testing.T) {
	c := cache.NewPrioCache(5)
	rand.Seed(time.Now().UnixNano())

	type kvp struct {
		k, v int
		p    float64
	}

	var kvps []kvp

	for i := 0; i < 100; i++ {
		e := kvp{
			rand.Int(),
			rand.Int(),
			rand.Float64()}
		kvps = append(kvps, e)
		c.Set(e.k, e.v, e.p)
	}

	sort.Slice(kvps, func(i, j int) bool {
		return kvps[i].p > kvps[j].p
	})
	var remained []kvp
	c.ForEach(func(entry *cache.PrioEntry) bool {
		remained = append(remained, kvp{
			entry.Key.(int),
			entry.Value.(int),
			entry.Priority})
		return true
	})

	sort.Slice(remained, func(i, j int) bool {
		return remained[i].p > remained[j].p
	})

	assert.Equal(t, kvps[:5], remained)
}
