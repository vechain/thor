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
	keyRevisionMap map[interface{}]*stack
}

type level struct {
	kvs     map[interface{}]interface{}
	journal []*journalEntry
}

func newLevel() *level {
	return &level{kvs: make(map[interface{}]interface{})}
}

// journalEntry entry of journal.
type journalEntry struct {
	key   interface{}
	value interface{}
}

// MapGetter defines getter method of map.
type MapGetter func(key interface{}) (value interface{}, exist bool, err error)

// New create an instance of StackedMap.
// src acts as source of data.
func New(src MapGetter) *StackedMap {
	return &StackedMap{
		src,
		stack{newLevel()},
		make(map[interface{}]*stack),
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
func (sm *StackedMap) Get(key interface{}) (interface{}, bool, error) {
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
func (sm *StackedMap) Put(key, value interface{}) {
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
func (sm *StackedMap) Journal(cb func(key, value interface{}) bool) {
	for _, lvl := range sm.mapStack {
		for _, entry := range lvl.(*level).journal {
			if !cb(entry.key, entry.value) {
				return
			}
		}
	}
}

// stack ops
type stack []interface{}

func (s *stack) pop() {
	*s = (*s)[:len(*s)-1]
}

func (s *stack) push(v interface{}) {
	*s = append(*s, v)
}
func (s stack) top() interface{} {
	return s[len(s)-1]
}
