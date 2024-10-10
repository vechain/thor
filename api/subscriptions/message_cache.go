package subscriptions

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/v2/chain"
)

type messageHandler = func(*chain.ExtendedBlock, *chain.Repository) ([]byte, error)

type messageCache struct {
	cache     *lru.Cache
	generator messageHandler
	mu        sync.RWMutex
}

func newMessageCache(handler messageHandler, cacheSize int) (*messageCache, error) {
	cache, err := lru.New(cacheSize)
	return &messageCache{
		cache:     cache,
		generator: handler,
	}, err
}

// GetOrAdd can be called by thousands of goroutines concurrently. The first goroutine that invokes it for a specific
// block will generate the message and store it in the cache. Subsequent goroutines will read the message from the cache.
func (mc *messageCache) GetOrAdd(block *chain.ExtendedBlock, repo *chain.Repository) ([]byte, error) {
	blockID := block.Header().ID().String()
	mc.mu.RLock()
	msg, ok := mc.cache.Get(blockID)
	mc.mu.RUnlock()
	if ok {
		return msg.([]byte), nil
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	msg, ok = mc.cache.Get(blockID)
	if ok {
		return msg.([]byte), nil
	}

	msg, err := mc.generator(block, repo)
	if err != nil {
		return nil, err
	}
	mc.cache.Add(blockID, msg)
	return msg.([]byte), nil
}
