package subscriptions

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/chain"
)

func handler(blk *chain.ExtendedBlock, _ *chain.Repository) ([]byte, error) {
	data := make(map[string]interface{})
	data["id"] = blk.Header().ID().String()
	return json.Marshal(blk)
}

func TestMessageCache_GetOrAdd(t *testing.T) {
	repo, generatedBlocks, _ := initChain(t)

	blk0 := generatedBlocks[0]
	blk1 := generatedBlocks[1]

	cache, err := newMessageCache(handler, 10)
	assert.NoError(t, err)

	counter := atomic.Int32{}
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		start := time.Now().Add(20 * time.Millisecond)
		go func() {
			defer wg.Done()
			time.Sleep(time.Until(start))
			_, added, err := cache.GetOrAdd(&chain.ExtendedBlock{Block: blk0}, repo)
			assert.NoError(t, err)
			if added {
				counter.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, counter.Load(), int32(1))

	_, added, err := cache.GetOrAdd(&chain.ExtendedBlock{Block: blk1}, repo)
	assert.NoError(t, err)
	assert.True(t, added)
	assert.Equal(t, cache.cache.Len(), 2)
}
