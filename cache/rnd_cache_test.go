package cache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/cache"
)

func TestRandCacheAddRemove(t *testing.T) {
	c := cache.NewRandCache(16)
	c.Set("key", "value")
	assert.Equal(t, 1, c.Len())

	v, b := c.Get("key")
	assert.Equal(t, "value", v)
	assert.True(t, b)

	assert.True(t, c.Remove("key"))

	_, b = c.Get("key")
	assert.False(t, b)
}

func TestRandCacheLimit(t *testing.T) {
	c := cache.NewRandCache(16)
	for i := 0; i < 100; i++ {
		c.Set(i, i)
	}

	assert.Equal(t, 16, c.Len())
}
