// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package cache

import (
	"math/rand"
	"sync"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// RandCache a simple cache which randomly evicts entries when
// length exceeds limit.
type RandCache struct {
	m     map[interface{}]*randEntry
	s     []*randEntry
	limit int
	lock  sync.Mutex
}

type randEntry struct {
	Entry
	index int
}

// NewRandCache create a new RandCache.
func NewRandCache(limit int) *RandCache {
	if limit < 1 {
		panic("invalid limit for RandCache")
	}
	return &RandCache{
		m:     make(map[interface{}]*randEntry),
		limit: limit,
	}
}

// Len returns count of entries in the cache.
func (rc *RandCache) Len() int {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	return len(rc.s)
}

// Set sets value for given key.
func (rc *RandCache) Set(key, value interface{}) {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	if ent, ok := rc.m[key]; ok {
		ent.Value = value
		return
	}
	ent := &randEntry{
		Entry: Entry{
			Key:   key,
			Value: value,
		},
		index: len(rc.s),
	}
	rc.m[key] = ent
	rc.s = append(rc.s, ent)

	if len(rc.s) > rc.limit {
		rc.randDrop()
	}
}

// Get get value for the given key.
func (rc *RandCache) Get(key interface{}) (interface{}, bool) {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	if ent, ok := rc.m[key]; ok {
		return ent.Value, true
	}
	return nil, false
}

// Contains returns whether the given key is contained.
func (rc *RandCache) Contains(key interface{}) bool {
	rc.lock.Lock()
	defer rc.lock.Unlock()
	_, ok := rc.m[key]
	return ok
}

// Remove removes key.
func (rc *RandCache) Remove(key interface{}) bool {
	rc.lock.Lock()
	defer rc.lock.Unlock()
	return rc.remove(key)
}

// Pick pick an entry randomly.
func (rc *RandCache) Pick() *Entry {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	if len(rc.s) == 0 {
		return nil
	}
	ent := rc.s[rand.Intn(len(rc.s))]
	cpy := ent.Entry
	return &cpy
}

// ForEach iterates all entries in the cache.
func (rc *RandCache) ForEach(cb func(*Entry) bool) bool {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	for _, ent := range rc.s {
		cpy := ent.Entry
		if !cb(&cpy) {
			return false
		}
	}
	return true
}

func (rc *RandCache) remove(key interface{}) bool {
	if ent, ok := rc.m[key]; ok {
		delete(rc.m, key)
		last := rc.s[len(rc.s)-1]
		rc.s[ent.index] = last
		last.index = ent.index
		rc.s = rc.s[:len(rc.s)-1]
		return true
	}
	return false
}

func (rc *RandCache) randDrop() {
	if len(rc.s) == 0 {
		return
	}
	ent := rc.s[rand.Intn(len(rc.s))]
	rc.remove(ent.Key)
}
