package w8cache

import (
	"container/heap"
	"sync"
)

// Entry entry of weight based cache.
type Entry struct {
	Key    interface{}
	Value  interface{}
	Weight float64
}

// W8Cache weight based cache.
type W8Cache struct {
	entryMap  map[interface{}]*entry
	entryHeap entryHeap
	limit     int
	evicted   func(*Entry)
	mu        sync.Mutex
}

// New create a new W8Cache instance.
func New(limit int, evicted func(*Entry)) *W8Cache {
	return &W8Cache{
		entryMap: make(map[interface{}]*entry),
		limit:    limit,
		evicted:  evicted,
	}
}

// Get get value and weight for given key.
func (c *W8Cache) Get(key interface{}) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entryMap[key]; ok {
		return entry.Value, true
	}
	return nil, false
}

// Set set or update value and weight for given key.
func (c *W8Cache) Set(key, value interface{}, weight float64) {
	c.mu.Lock()
	evicted := c.set(key, value, weight)
	c.mu.Unlock()

	if evicted != nil && c.evicted != nil {
		c.evicted(evicted)
	}
}

func (c *W8Cache) set(key, value interface{}, weight float64) *Entry {
	if entry, ok := c.entryMap[key]; ok {
		entry.Value = value
		entry.Weight = weight
		heap.Fix(&c.entryHeap, entry.index)
		return nil
	}

	newEntry := &entry{
		Entry: Entry{
			Key:    key,
			Value:  value,
			Weight: weight,
		},
	}
	heap.Push(&c.entryHeap, newEntry)
	c.entryMap[key] = newEntry
	if len(c.entryHeap) > c.limit {
		return c.popWorst()
	}
	return nil
}

// Remove remove the given key.
func (c *W8Cache) Remove(key interface{}) *Entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entryMap[key]; ok {
		delete(c.entryMap, key)
		heap.Remove(&c.entryHeap, entry.index)
		return &entry.Entry
	}
	return nil
}

// PopWorst pop the least weight entry.
func (c *W8Cache) PopWorst() *Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.popWorst()
}

func (c *W8Cache) popWorst() *Entry {
	if len(c.entryHeap) == 0 {
		return nil
	}
	popped := heap.Pop(&c.entryHeap).(*entry)
	delete(c.entryMap, popped.Key)
	return &popped.Entry
}

// Count returns count of value.
func (c *W8Cache) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entryHeap)
}

// All dumps all entries.
func (c *W8Cache) All() []*Entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	all := make([]*Entry, 0, len(c.entryHeap))
	for _, entry := range c.entryHeap {
		cpy := entry.Entry
		all = append(all, &cpy)
	}
	return all
}

type entry struct {
	Entry
	index int
}

type entryHeap []*entry

func (h entryHeap) Len() int           { return len(h) }
func (h entryHeap) Less(i, j int) bool { return h[i].Weight < h[j].Weight }
func (h entryHeap) Swap(i, j int) {
	h[i].index = j
	h[j].index = i
	h[i], h[j] = h[j], h[i]
}

func (h *entryHeap) Push(value interface{}) {
	ent := value.(*entry)
	ent.index = len(*h)
	*h = append(*h, ent)
}

func (h *entryHeap) Pop() interface{} {
	n := len(*h)
	ent := (*h)[n-1]
	ent.index = -1
	*h = (*h)[:n-1]
	return ent
}
