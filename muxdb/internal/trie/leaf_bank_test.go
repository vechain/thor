// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/trie"
)

func TestLeafbank(t *testing.T) {
	engine := newEngine()
	lb := NewLeafBank(engine, 2, 100)
	name := "the trie"

	t.Run("empty state", func(t *testing.T) {
		key := []byte("key")
		rec, err := lb.Lookup(name, key)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, &LeafRecord{}, rec)
	})

	t.Run("update and lookup", func(t *testing.T) {
		u := lb.NewUpdater(name, 100)
		for i := 0; i < 10; i++ {
			if err := u.Update([]byte(strconv.Itoa(i)), &trie.Leaf{Value: []byte(strconv.Itoa(i))}); err != nil {
				t.Fatal(err)
			}
		}
		if err := u.Commit(); err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 10; i++ {
			rec, err := lb.Lookup(name, []byte(strconv.Itoa(i)))
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, &LeafRecord{
				Leaf: &trie.Leaf{
					Value: []byte(strconv.Itoa(i)),
				},
				CommitNum: 100,
			}, rec)
		}
	})

	t.Run("lookup never seen", func(t *testing.T) {
		rec, err := lb.Lookup(name, []byte(strconv.Itoa(11)))
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, &LeafRecord{
			Leaf:      nil,
			CommitNum: 100,
		}, rec)
	})

	t.Run("lookup touched", func(t *testing.T) {
		// mark
		if err := lb.Touch(engine, name, []byte(strconv.Itoa(1))); err != nil {
			t.Fatal(err)
		}
		// recreate to drop cache
		lb = NewLeafBank(engine, 2, 100)

		rec, err := lb.Lookup(name, []byte(strconv.Itoa(1)))
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, &LeafRecord{
			Leaf:      nil,
			CommitNum: math.MaxUint32,
		}, rec)
	})
}
