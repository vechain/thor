// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
)

func handler(blk *block.Block) func() ([]byte, error) {
	return func() ([]byte, error) {
		data := make(map[string]interface{})
		data["id"] = blk.Header().ID().String()
		return json.Marshal(data)
	}
}

func TestMessageCache_GetOrAdd(t *testing.T) {
	_, generatedBlocks, _ := initChain(t)

	blk0 := generatedBlocks[0]
	blk1 := generatedBlocks[1]

	cache := newMessageCache(10)

	counter := atomic.Int32{}
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		start := time.Now().Add(20 * time.Millisecond)
		go func() {
			defer wg.Done()
			time.Sleep(time.Until(start))
			_, added, err := cache.GetOrAdd(blk0.Header().ID(), handler(blk0))
			assert.NoError(t, err)
			if added {
				counter.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, counter.Load(), int32(1))

	_, added, err := cache.GetOrAdd(blk1.Header().ID(), handler(blk1))
	assert.NoError(t, err)
	assert.True(t, added)
	assert.Equal(t, cache.cache.Len(), 2)
}

func TestNewMessageCache(t *testing.T) {
	cache := newMessageCache(1001)
	assert.Equal(t, cache.cache.Len(), 1000)
}
