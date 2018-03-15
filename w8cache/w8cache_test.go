package w8cache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/w8cache"
)

func TestW8Cache(t *testing.T) {
	var evicted []*w8cache.Entry
	cache := w8cache.New(2, func(e *w8cache.Entry) {
		evicted = append(evicted, e)
	})

	cache.Set("foo", "foo", 2)
	cache.Set("bar", "bar", 1)
	cache.Set("baz", "baz", 3)

	assert.Equal(t, []*w8cache.Entry{&w8cache.Entry{
		Key:    "bar",
		Value:  "bar",
		Weight: float64(1),
	}}, evicted)

	v, ok := cache.Get("foo")
	assert.Equal(t, "foo", v)
	assert.True(t, ok)

	v, ok = cache.Get("bar")
	assert.Equal(t, nil, v)
	assert.False(t, ok)
}
