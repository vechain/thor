// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
)

type message struct {
	id string
}

func handler(blk *block.Block) func() (message, error) {
	return func() (message, error) {
		msg := message{
			id: blk.Header().ID().String(),
		}
		return msg, nil
	}
}

func TestMessageCache_GetOrAdd(t *testing.T) {
	thorChain := initChain(t)

	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)

	blk0 := allBlocks[0]
	blk1 := allBlocks[1]

	cache := newMessageCache[message](10)

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
