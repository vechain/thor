package stackedmap

// StackedMap maintains maps in a stack.
// Each map inherits key/value of map that is at lower level.
// It acts as a map with save-restore/snapshot-revert manner.
type StackedMap struct {
	src            MapGetter
	mapStack       stack
	keyRevisionMap map[interface{}]*stack
}

type _map map[interface{}]interface{}

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
	sm.mapStack.push(make(_map))
	return len(sm.mapStack) - 1
}

// Pop pop the map at top of stack.
// It will revert all Put operations since last Push.
func (sm *StackedMap) Pop() {
	// pop key revision
	top := sm.mapStack.top().(_map)
	for key := range top {
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
		m := sm.mapStack[revs.top().(int)].(_map)
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return sm.src(key)
}

// Put puts key value into map at stack top.
// It will panic if stack is empty.
func (sm *StackedMap) Put(key, value interface{}) {
	top := sm.mapStack.top().(_map)
	top[key] = value

	// records key revision for fast access
	rev := len(sm.mapStack) - 1
	if revs, ok := sm.keyRevisionMap[key]; ok {
		revs.push(rev)
	} else {
		sm.keyRevisionMap[key] = &stack{rev}
	}
}

// Puts returns all Put key-value pairs。
func (sm *StackedMap) Puts() map[interface{}]interface{} {
	puts := make(map[interface{}]interface{})
	for key, revs := range sm.keyRevisionMap {
		rev := revs.top().(int)
		puts[key] = sm.mapStack[rev].(_map)[key]
	}
	return puts
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
