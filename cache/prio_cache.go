// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package cache

import (
	"container/heap"
	"sync"
)

// PrioCache a cache holds entries with priority.
// If len of cache reaches limit, the entry has lowest priority will be evicted.
type PrioCache struct {
	m     map[interface{}]*prioEntry
	s     prioEntries
	limit int
	lock  sync.Mutex
}

// NewPrioCache create a new PrioCache.
func NewPrioCache(limit int) *PrioCache {
	if limit < 1 {
		panic("invalid limit for PrioCache")
	}
	return &PrioCache{
		m:     make(map[interface{}]*prioEntry),
		limit: limit,
	}
}

// Len returns count of entries in the cache.
func (pc *PrioCache) Len() int {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	return len(pc.s)
}

// Set set value and priority for given key.
func (pc *PrioCache) Set(key, value interface{}, priority float64) {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	if ent, ok := pc.m[key]; ok {
		ent.Value = value
		ent.Priority = priority
		heap.Fix(&pc.s, ent.index)
		return
	}
	ent := &prioEntry{
		PrioEntry: PrioEntry{
			Entry: Entry{
				Key:   key,
				Value: value,
			},
			Priority: priority,
		},
	}
	heap.Push(&pc.s, ent)
	pc.m[key] = ent

	if len(pc.s) > pc.limit {
		pc.popLowest()
	}
}

// Get retrieves value for given key.
func (pc *PrioCache) Get(key interface{}) (interface{}, float64, bool) {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	if ent, ok := pc.m[key]; ok {
		return ent.Value, ent.Priority, true
	}
	return nil, 0, false
}

// Contains returns whether the given key is contained.
func (pc *PrioCache) Contains(key interface{}) bool {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	_, ok := pc.m[key]
	return ok
}

// Remove removes the given key, and returns the removed entry if any.
func (pc *PrioCache) Remove(key interface{}) *PrioEntry {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	if ent, ok := pc.m[key]; ok {
		delete(pc.m, key)
		heap.Remove(&pc.s, ent.index)
		return &ent.PrioEntry
	}
	return nil
}

// ForEach iterates all cache entries.
func (pc *PrioCache) ForEach(cb func(*PrioEntry) bool) bool {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	for _, ent := range pc.s {
		cpy := ent.PrioEntry
		if !cb(&cpy) {
			return false
		}
	}
	return true
}

func (pc *PrioCache) popLowest() {
	if len(pc.s) == 0 {
		return
	}
	ent := heap.Pop(&pc.s).(*prioEntry)
	delete(pc.m, ent.Key)
}

// PrioEntry cache entry with priority.
type PrioEntry struct {
	Entry
	Priority float64
}

type prioEntry struct {
	PrioEntry
	index int
}

type prioEntries []*prioEntry

func (h prioEntries) Len() int           { return len(h) }
func (h prioEntries) Less(i, j int) bool { return h[i].Priority < h[j].Priority }
func (h prioEntries) Swap(i, j int) {
	h[i].index = j
	h[j].index = i
	h[i], h[j] = h[j], h[i]
}

func (h *prioEntries) Push(value interface{}) {
	ent := value.(*prioEntry)
	ent.index = len(*h)
	*h = append(*h, ent)
}

func (h *prioEntries) Pop() interface{} {
	n := len(*h)
	ent := (*h)[n-1]
	ent.index = -1
	*h = (*h)[:n-1]
	return ent
}
