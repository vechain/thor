package stackedmap_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/fortest"
	"github.com/vechain/thor/stackedmap"
)

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
			assert.Equal(fortest.Multi(sm.Get(test.getKey)), test.getReturn)
		}
	}
}

func TestStackedMapPuts(t *testing.T) {
	assert := assert.New(t)
	sm := stackedmap.New(func(key interface{}) (interface{}, bool) {
		return nil, false
	})

	kvs := []*stackedmap.JournalEntry{
		{Key: "a", Value: "b"},
		{Key: "a1", Value: "b1"},
		{Key: "a2", Value: "b2"},
		{Key: "a3", Value: "b3"},
		{Key: "a4", Value: "b4"},
	}

	for _, kv := range kvs {
		sm.Push()
		sm.Put(kv.Key, kv.Value)
	}
	assert.Equal(sm.Journal(), kvs)
}
