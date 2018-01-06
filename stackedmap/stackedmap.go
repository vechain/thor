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
	journal []*JournalEntry
}

func newLevel() *level {
	return &level{kvs: make(map[interface{}]interface{})}
}

// JournalEntry entry of journal.
type JournalEntry struct {
	Key   interface{}
	Value interface{}
}

// MapGetter defines getter method of map.
type MapGetter func(key interface{}) (value interface{}, exist bool)

// New create an instance of StackedMap.
// src acts as source of data.
func New(src MapGetter) *StackedMap {
	return &StackedMap{
		src,
		stack{},
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
func (sm *StackedMap) Get(key interface{}) (interface{}, bool) {
	if revs, ok := sm.keyRevisionMap[key]; ok {
		lvl := sm.mapStack[revs.top().(int)].(*level)
		if v, ok := lvl.kvs[key]; ok {
			return v, true
		}
	}
	return sm.src(key)
}

// Put puts key value into map at stack top.
// It will panic if stack is empty.
func (sm *StackedMap) Put(key, value interface{}) {
	top := sm.mapStack.top().(*level)
	top.kvs[key] = value
	top.journal = append(top.journal, &JournalEntry{Key: key, Value: value})

	// records key revision for fast access
	rev := len(sm.mapStack) - 1
	if revs, ok := sm.keyRevisionMap[key]; ok {
		revs.push(rev)
	} else {
		sm.keyRevisionMap[key] = &stack{rev}
	}
}

// Journal returns journal of all Put operations.
func (sm *StackedMap) Journal() (j []*JournalEntry) {
	for _, lvl := range sm.mapStack {
		j = append(j, lvl.(*level).journal...)
	}
	return
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
