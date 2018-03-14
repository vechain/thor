package cache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/cache"
)

func TestW8(t *testing.T) {
	w8 := cache.NewW8(2)
	w8.Set("a", 1, 1)
	w8.Set("b", 2, 2)
	evicted := w8.Set("c", 3, 3)
	assert.NotNil(t, evicted)
	assert.Equal(t, "a", evicted.Key)
	assert.Equal(t, 1, evicted.Value)
	assert.Equal(t, float64(1), evicted.Weight)
	assert.Equal(t, 2, w8.Count())
}
