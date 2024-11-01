package subscriptions

import (
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/v2/thor"
)

// messageCache is a generic cache that stores messages of any type.
type messageCache[T any] struct {
	cache *lru.Cache
	mu    sync.RWMutex
}

// newMessageCache creates a new messageCache with the specified cache size.
func newMessageCache[T any](cacheSize uint32) *messageCache[T] {
	if cacheSize > 1000 {
		cacheSize = 1000
	}
	if cacheSize == 0 {
		cacheSize = 1
	}
	cache, err := lru.New(int(cacheSize))
	if err != nil {
		// lru.New only throws an error if the number is less than 1
		panic(fmt.Errorf("failed to create message cache: %v", err))
	}
	return &messageCache[T]{
		cache: cache,
	}
}

// GetOrAdd returns the message of the block. If the message is not in the cache,
// it will generate the message and add it to the cache. The second return value
// indicates whether the message is newly generated.
func (mc *messageCache[T]) GetOrAdd(id thor.Bytes32, createMessage func() (T, error)) (T, bool, error) {
	blockID := id.String()
	mc.mu.RLock()
	msg, ok := mc.cache.Get(blockID)
	mc.mu.RUnlock()
	if ok {
		return msg.(T), false, nil
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	msg, ok = mc.cache.Get(blockID)
	if ok {
		return msg.(T), false, nil
	}

	newMsg, err := createMessage()
	if err != nil {
		var zero T
		return zero, false, err
	}
	mc.cache.Add(blockID, newMsg)
	return newMsg, true, nil
}
