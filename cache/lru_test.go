package cache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	. "github.com/vechain/thor/cache"
)

func TestLRU(t *testing.T) {
	assert := assert.New(t)
	lru, _ := NewLRU(10)
	assert.Equal(lru.GetOrLoad("foo", func() interface{} {
		return "bar"
	}), "bar")

	v, _ := lru.Get("foo")
	assert.Equal(v, "bar")
}
