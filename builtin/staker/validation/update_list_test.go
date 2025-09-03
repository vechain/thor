// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestUpdateList(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	list := newUpdateList(sctx, slotUpdateHead, slotUpdateTail, slotUpdatePrev, slotUpdateNext)

	// zero address behavior
	has, err := list.contains(thor.Address{})
	assert.NoError(t, err)
	assert.False(t, has)

	assert.NoError(t, list.Add(thor.Address{}))
	assert.NoError(t, list.Remove(thor.Address{}))

	addresses := []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(addresses))

	addr1 := thor.Address{1}
	addr2 := thor.Address{2}

	head, err := list.head.Get()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())

	tail, err := list.tail.Get()
	assert.NoError(t, err)
	assert.True(t, tail.IsZero())

	err = list.Add(addr1)
	assert.NoError(t, err)

	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(addresses))
	assert.Equal(t, addr1, addresses[0])

	has, err = list.contains(addr1)
	assert.NoError(t, err)
	assert.True(t, has)

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, head)

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, tail)

	// add again should be no-op
	err = list.Add(addr1)
	assert.NoError(t, err)

	has, err = list.contains(addr1)
	assert.NoError(t, err)
	assert.True(t, has)

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, head)

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, tail)

	// remove addr1
	err = list.Remove(addr1)
	assert.NoError(t, err)

	has, err = list.contains(addr1)
	assert.NoError(t, err)
	assert.False(t, has)

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.True(t, tail.IsZero())

	// add addr2
	err = list.Add(addr2)
	assert.NoError(t, err)

	has, err = list.contains(addr2)
	assert.NoError(t, err)
	assert.True(t, has)

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr2, head)

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr2, tail)

	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(addresses))
	assert.Equal(t, addr2, addresses[0])

	// add addr1
	err = list.Add(addr1)
	assert.NoError(t, err)

	has, err = list.contains(addr1)
	assert.NoError(t, err)
	assert.True(t, has)

	has, err = list.contains(addr2)
	assert.NoError(t, err)
	assert.True(t, has)

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr2, head)

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, tail)

	// remove addr2
	err = list.Remove(addr2)
	assert.NoError(t, err)

	has, err = list.contains(addr2)
	assert.NoError(t, err)
	assert.False(t, has)

	has, err = list.contains(addr1)
	assert.NoError(t, err)
	assert.True(t, has)

	// remove addr1
	err = list.Remove(addr1)
	assert.NoError(t, err)

	has, err = list.contains(addr1)
	assert.NoError(t, err)
	assert.False(t, has)

	has, err = list.contains(addr2)
	assert.NoError(t, err)
	assert.False(t, has)

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.True(t, tail.IsZero())

	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(addresses))
}

func TestUpdateListRemove(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	list := newUpdateList(sctx, slotUpdateHead, slotUpdateTail, slotUpdatePrev, slotUpdateNext)

	// zero address behavior
	assert.NoError(t, list.Remove(thor.Address{4}))

	addr1 := thor.Address{1}
	addr2 := thor.Address{2}
	addr3 := thor.Address{3}
	addr4 := thor.Address{4}

	assert.NoError(t, list.Add(addr1))
	assert.NoError(t, list.Add(addr2))
	assert.NoError(t, list.Add(addr3))

	has, err := list.contains(addr1)
	assert.NoError(t, err)
	assert.True(t, has)

	has, err = list.contains(addr2)
	assert.NoError(t, err)
	assert.True(t, has)

	has, err = list.contains(addr3)
	assert.NoError(t, err)
	assert.True(t, has)

	head, err := list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, head)

	tail, err := list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr3, tail)

	// remove addr2
	assert.NoError(t, list.Remove(addr2))

	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, head)

	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr3, tail)

	assert.NoError(t, list.Add(addr4))
	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr4, tail)

	err = list.Remove(addr4)
	assert.NoError(t, err)
	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr3, tail)
	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, head)

	err = list.Remove(addr3)
	assert.NoError(t, err)
	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, tail)
	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, addr1, head)

	err = list.Remove(addr1)
	assert.NoError(t, err)
	tail, err = list.tail.Get()
	assert.NoError(t, err)
	assert.True(t, tail.IsZero())
	head, err = list.head.Get()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())
}

func TestUpdateListIterate(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	list := newUpdateList(sctx, slotUpdateHead, slotUpdateTail, slotUpdatePrev, slotUpdateNext)

	addr1 := thor.Address{1}
	addr2 := thor.Address{2}
	addr3 := thor.Address{3}
	addr4 := thor.Address{4}

	assert.NoError(t, list.Add(addr1))
	assert.NoError(t, list.Add(addr2))
	assert.NoError(t, list.Add(addr3))
	assert.NoError(t, list.Add(addr4))

	addresses := []thor.Address{}
	err := list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 4, len(addresses))
	assert.Equal(t, addr1, addresses[0])
	assert.Equal(t, addr2, addresses[1])
	assert.Equal(t, addr3, addresses[2])
	assert.Equal(t, addr4, addresses[3])

	assert.NoError(t, list.Remove(addr2))
	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(addresses))
	assert.Equal(t, addr1, addresses[0])
	assert.Equal(t, addr3, addresses[1])
	assert.Equal(t, addr4, addresses[2])

	assert.NoError(t, list.Remove(addr4))
	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(addresses))
	assert.Equal(t, addr1, addresses[0])
	assert.Equal(t, addr3, addresses[1])

	assert.NoError(t, list.Remove(addr1))
	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(addresses))
	assert.Equal(t, addr3, addresses[0])

	assert.NoError(t, list.Remove(addr3))
	addresses = []thor.Address{}
	err = list.Iterate(func(address thor.Address) error {
		addresses = append(addresses, address)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(addresses))
}
