package cache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/cache"
)

func TestLRU(t *testing.T) {
	assert := assert.New(t)
	lru, _ := cache.NewLRU(10)
	v, _ := lru.GetOrLoad("foo", func(interface{}) (interface{}, error) {
		return "bar", nil
	})
	assert.Equal(v, "bar")

	v, _ = lru.Get("foo")
	assert.Equal(v, "bar")
}
