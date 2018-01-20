package stackedmap_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/stackedmap"
)

func M(a ...interface{}) []interface{} {
	return a
}
func TestStackedMap(t *testing.T) {

	assert := assert.New(t)
	src := make(map[string]string)
	src["foo"] = "bar"

	sm := stackedmap.New(func(key interface{}) (interface{}, bool) {
		v, r := src[key.(string)]
		return v, r
	})

	tests := []struct {
		f         func()
		depth     int
		putKey    string
		putValue  string
		getKey    string
		getReturn []interface{}
	}{
		{func() {}, 0, "", "", "foo", []interface{}{"bar", true}},
		{func() { sm.Push() }, 1, "foo", "baz", "foo", []interface{}{"baz", true}},
		{func() { sm.Push() }, 2, "foo", "qux", "foo", []interface{}{"qux", true}},
		{func() { sm.Pop() }, 1, "", "", "foo", []interface{}{"baz", true}},
		{func() { sm.Pop() }, 0, "", "", "foo", []interface{}{"bar", true}},

		{func() { sm.Push(); sm.Push() }, 2, "", "", "", nil},
		{func() { sm.PopTo(0) }, 0, "", "", "", nil},
	}

	for _, test := range tests {
		test.f()
		assert.Equal(sm.Depth(), test.depth)
		if test.putKey != "" {
			sm.Put(test.putKey, test.putValue)
		}
		if test.getKey != "" {
			assert.Equal(M(sm.Get(test.getKey)), test.getReturn)
		}
	}
}

func TestStackedMapPuts(t *testing.T) {
	assert := assert.New(t)
	sm := stackedmap.New(func(key interface{}) (interface{}, bool) {
		return nil, false
	})

	kvs := []struct {
		k, v string
	}{
		{"a", "b"},
		{"a", "b"},
		{"a1", "b1"},
		{"a2", "b2"},
		{"a3", "b3"},
		{"a4", "b4"},
	}

	for _, kv := range kvs {
		sm.Push()
		sm.Put(kv.k, kv.v)
	}
	i := 0
	sm.Journal(func(k, v interface{}) bool {
		assert.Equal(k, kvs[i].k)
		assert.Equal(v, kvs[i].v)
		i++
		return true
	})
	assert.Equal(len(kvs), i, "Journal traverse should abort")

	i = 0
	sm.Journal(func(k, v interface{}) bool {
		i++
		return false
	})

	assert.Equal(1, i, "Journal traverse should abort")
}
