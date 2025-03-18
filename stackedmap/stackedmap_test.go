// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package stackedmap_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/stackedmap"
)

func M(a ...any) []any {
	return a
}
func TestStackedMap(t *testing.T) {
	assert := assert.New(t)
	src := make(map[string]string)
	src["foo"] = "bar"

	sm := stackedmap.New(func(key any) (any, bool, error) {
		v, r := src[key.(string)]
		return v, r, nil
	})

	tests := []struct {
		f         func()
		depth     int
		putKey    string
		putValue  string
		getKey    string
		getReturn []any
	}{
		{func() {}, 1, "", "", "foo", []any{"bar", true, nil}},
		{func() { sm.Push() }, 2, "foo", "baz", "foo", []any{"baz", true, nil}},
		{func() {}, 2, "foo", "baz1", "foo", []any{"baz1", true, nil}},
		{func() { sm.Push() }, 3, "foo", "qux", "foo", []any{"qux", true, nil}},
		{func() { sm.Pop() }, 2, "", "", "foo", []any{"baz1", true, nil}},
		{func() { sm.Pop() }, 1, "", "", "foo", []any{"bar", true, nil}},

		{func() { sm.Push(); sm.Push() }, 3, "", "", "", nil},
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
	sm := stackedmap.New(func(_ any) (any, bool, error) {
		return nil, false, nil
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
	sm.Journal(func(k, v any) bool {
		assert.Equal(k, kvs[i].k)
		assert.Equal(v, kvs[i].v)
		i++
		return true
	})
	assert.Equal(len(kvs), i, "Journal traverse should abort")

	i = 0
	sm.Journal(func(_, _ any) bool {
		i++
		return false
	})

	assert.Equal(1, i, "Journal traverse should abort")
}
