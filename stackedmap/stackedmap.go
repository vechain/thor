// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package stackedmap

// StackedMap maintains maps in a stack.
// Each map inherits key/value of map that is at lower level.
// It acts as a map with save-restore/snapshot-revert manner.
type StackedMap struct {
	src            MapGetter
	mapStack       stack
	keyRevisionMap map[any]*stack
}

type level struct {
	kvs     map[any]any
	journal []*journalEntry
}

func newLevel() *level {
	return &level{kvs: make(map[any]any)}
}

// journalEntry entry of journal.
type journalEntry struct {
	key   any
	value any
}

// MapGetter defines getter method of map.
type MapGetter func(key any) (value any, exist bool, err error)

// New create an instance of StackedMap.
// src acts as source of data.
func New(src MapGetter) *StackedMap {
	return &StackedMap{
		src,
		stack{newLevel()},
		make(map[any]*stack),
	}
}

// Depth returns depth of stack.
func (sm *StackedMap) Depth() int {
	return len(sm.mapStack)
}

// Push pushes a new map on stack.
// It returns stack depth before push.
func (sm *StackedMap) Push() int {
	sm.mapStack.push(newLevel())
	return len(sm.mapStack) - 1
}

// Pop pop the map at top of stack.
// It will revert all Put operations since last Push.
func (sm *StackedMap) Pop() {
	// pop key revision
	top := sm.mapStack.top().(*level)
	for key := range top.kvs {
		revs := sm.keyRevisionMap[key]
		revs.pop()
		if len(*revs) == 0 {
			delete(sm.keyRevisionMap, key)
		}
	}
	sm.mapStack.pop()
}

// PopTo pop maps until stack depth reaches depth.
func (sm *StackedMap) PopTo(depth int) {
	for len(sm.mapStack) > depth {
		sm.Pop()
	}
}

// Get gets value for given key.
// The second return value indicates whether the given key is found.
func (sm *StackedMap) Get(key any) (any, bool, error) {
	if revs, ok := sm.keyRevisionMap[key]; ok {
		lvl := sm.mapStack[revs.top().(int)].(*level)
		if v, ok := lvl.kvs[key]; ok {
			return v, true, nil
		}
	}
	return sm.src(key)
}

// Put puts key value into map at stack top.
// It will panic if stack is empty.
func (sm *StackedMap) Put(key, value any) {
	top := sm.mapStack.top().(*level)
	top.kvs[key] = value
	top.journal = append(top.journal, &journalEntry{key: key, value: value})

	// records key revision for fast access
	rev := len(sm.mapStack) - 1
	if revs, ok := sm.keyRevisionMap[key]; ok {
		if revs.top().(int) != rev {
			revs.push(rev)
		}
	} else {
		sm.keyRevisionMap[key] = &stack{rev}
	}
}

// Journal traverse journal entries of all Put operations.
// The traverse will abort if the callback func returns false.
func (sm *StackedMap) Journal(cb func(key, value any) bool) {
	for _, lvl := range sm.mapStack {
		for _, entry := range lvl.(*level).journal {
			if !cb(entry.key, entry.value) {
				return
			}
		}
	}
}

// stack ops
type stack []any

func (s *stack) pop() {
	*s = (*s)[:len(*s)-1]
}

func (s *stack) push(v any) {
	*s = append(*s, v)
}
func (s stack) top() any {
	return s[len(s)-1]
}
