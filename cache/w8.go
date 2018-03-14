package cache

import (
	"container/heap"
)

// W8 weight based cache.
type W8 struct {
	entryMap  map[interface{}]*wentry
	entryHeap wheap
	maxCount  int
}

// NewW8 create a new instance.
func NewW8(maxCount int) *W8 {
	return &W8{
		entryMap: make(map[interface{}]*wentry),
		maxCount: maxCount,
	}
}

// Get get value and weight for given key.
func (c *W8) Get(key interface{}) *struct {
	Value  interface{}
	Weight float64
} {
	if entry, ok := c.entryMap[key]; ok {
		return &struct {
			Value  interface{}
			Weight float64
		}{
			entry.value,
			entry.weight,
		}
	}
	return nil
}

// Set set or update value and weight for given key.
// Returns the evicted value if the count value exeeds max count.
func (c *W8) Set(key, value interface{}, weight float64) (evicted *struct {
	Key    interface{}
	Value  interface{}
	Weight float64
}) {
	if entry, ok := c.entryMap[key]; ok {
		entry.value = value
		entry.weight = weight
		heap.Fix(&c.entryHeap, entry.index)
		return nil
	}

	newEntry := &wentry{
		key:    key,
		value:  value,
		weight: weight,
	}
	heap.Push(&c.entryHeap, newEntry)
	c.entryMap[key] = newEntry
	if len(c.entryHeap) > c.maxCount {
		popped := heap.Pop(&c.entryHeap).(*wentry)
		delete(c.entryMap, popped.key)
		return &struct {
			Key    interface{}
			Value  interface{}
			Weight float64
		}{popped.key, popped.value, popped.weight}
	}
	return nil
}

// Count returns count of value.
func (c *W8) Count() int {
	return len(c.entryHeap)
}

type wentry struct {
	key    interface{}
	value  interface{}
	weight float64
	index  int
}

type wheap []*wentry

func (h wheap) Len() int           { return len(h) }
func (h wheap) Less(i, j int) bool { return h[i].weight < h[j].weight }
func (h wheap) Swap(i, j int) {
	h[i].index = j
	h[j].index = i
	h[i], h[j] = h[j], h[i]
}

func (h *wheap) Push(value interface{}) {
	ent := value.(*wentry)
	ent.index = len(*h)
	*h = append(*h, ent)
}

func (h *wheap) Pop() interface{} {
	n := len(*h)
	ent := (*h)[n-1]
	ent.index = -1
	*h = (*h)[:n-1]
	return ent
}
