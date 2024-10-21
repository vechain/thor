// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/v2/thor"
)

type messageCache struct {
	cache *lru.Cache
	mu    sync.RWMutex
}

func newMessageCache(cacheSize uint32) *messageCache {
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
	return &messageCache{
		cache: cache,
	}
}

// GetOrAdd returns the message of the block, if the message is not in the cache, it will generate the message and add it to the cache.
// The second return value indicates whether the message is newly generated.
func (mc *messageCache) GetOrAdd(id thor.Bytes32, createMessage func() ([]byte, error)) ([]byte, bool, error) {
	blockID := id.String()
	mc.mu.RLock()
	msg, ok := mc.cache.Get(blockID)
	mc.mu.RUnlock()
	if ok {
		return msg.([]byte), false, nil
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	msg, ok = mc.cache.Get(blockID)
	if ok {
		return msg.([]byte), false, nil
	}

	msg, err := createMessage()
	if err != nil {
		return nil, false, err
	}
	mc.cache.Add(blockID, msg)
	return msg.([]byte), true, nil
}
