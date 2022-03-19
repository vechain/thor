// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/trie"
)

func TestLeafbank(t *testing.T) {
	engine := newEngine()
	space := byte(2)
	slotCap := 10
	lb := NewLeafBank(engine, space, slotCap)
	name := "the trie"

	t.Run("empty state", func(t *testing.T) {
		key := []byte("key")
		rec, err := lb.Lookup(name, key)
		assert.NoError(t, err)
		assert.Equal(t, &LeafRecord{}, rec)
	})

	t.Run("update and lookup", func(t *testing.T) {
		u, err := lb.NewUpdater(name, 0, 100)
		assert.Nil(t, err)
		for i := 0; i < 10; i++ {
			if err := u.Update([]byte(strconv.Itoa(i)), &trie.Leaf{Value: []byte(strconv.Itoa(i))}, 10); err != nil {
				t.Fatal(err)
			}
		}
		if err := u.Commit(); err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 10; i++ {
			rec, err := lb.Lookup(name, []byte(strconv.Itoa(i)))
			assert.NoError(t, err)
			assert.Equal(t, &LeafRecord{
				Leaf:          &trie.Leaf{Value: []byte(strconv.Itoa(i))},
				CommitNum:     10,
				SlotCommitNum: 100,
			}, rec)
		}
	})

	t.Run("lookup never seen", func(t *testing.T) {
		rec, err := lb.Lookup(name, []byte(strconv.Itoa(11)))
		assert.NoError(t, err)

		assert.Equal(t, &LeafRecord{Leaf: &trie.Leaf{}, SlotCommitNum: 100}, rec)
	})

	t.Run("lookup deleted", func(t *testing.T) {
		// mark
		err := lb.LogDeletions(engine, name, []string{strconv.Itoa(1)}, 101)
		assert.Nil(t, err)

		u, err := lb.NewUpdater(name, 100, 101)
		assert.Nil(t, err)

		err = u.Commit()
		assert.Nil(t, err)

		// recreate to drop cache
		lb = NewLeafBank(engine, space, slotCap)

		rec, err := lb.Lookup(name, []byte(strconv.Itoa(1)))
		assert.NoError(t, err)
		assert.Equal(t, &LeafRecord{SlotCommitNum: 101}, rec)
	})
}
