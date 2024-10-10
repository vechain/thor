// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

// GetOrAdd returns the message of the block, if the message is not in the cache, it will generate the message and add it to the cache.
// The second return value indicates whether the message is newly generated.
func (mc *messageCache) GetOrAdd(block *chain.ExtendedBlock, repo *chain.Repository) ([]byte, bool, error) {
	blockID := block.Header().ID().String()
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

	msg, err := mc.generator(block, repo)
	if err != nil {
		return nil, false, err
	}
	mc.cache.Add(blockID, msg)
	return msg.([]byte), true, nil
}
