// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package linkedlist

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func Test_LinkedList_Remove_NonExistent(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	// Try to remove non-existent id
	if err := linkedList.Remove(id3); err != nil {
		t.Fatalf("expected no error when removing non-existent id, got: %v", err)
	}

	// Verify head is still id1
	head, err := linkedList.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// Verify removed-node pointers are cleared
	nextPtr, err := linkedList.next.Get(id2)
	assert.NoError(t, err)
	assert.True(t, nextPtr.IsZero(), "expected next[id2] to be cleared")
	prevPtr, err := linkedList.prev.Get(id1)
	assert.NoError(t, err)
	assert.True(t, prevPtr.IsZero(), "expected prev[id1] to be cleared")

	// Try to remove non-existent id
	if err := linkedList.Remove(id3); err != nil {
		t.Fatalf("expected no error when removing non-existent id, got: %v", err)
	}

	// Head unchanged
	head, err = linkedList.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// Tail unchanged (i.e. Next(id2) is zero)
	next, err := linkedList.Next(id2)
	assert.NoError(t, err)
	assert.True(t, next.IsZero())

	// Length unchanged
	length, err := linkedList.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), length)

	// Link from id1 → id2 still intact
	next, err = linkedList.Next(id1)
	assert.NoError(t, err)
	assert.Equal(t, id2, next)
}

func Test_LinkedList_Remove_NegativeTests(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	id1 := thor.Address{}
	id2 := thor.Address{}

	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	assert.Nil(t, linkedList.Remove(thor.Address{}))
}

func Test_LinkedList_Remove(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	// Verify head is id1
	head, err := linkedList.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// Remove id1
	if err := linkedList.Remove(id1); err != nil {
		t.Fatalf("failed to remove id1: %v", err)
	}

	// Verify head is now id2
	head, err = linkedList.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id2, head)

	// Remove id2
	if err := linkedList.Remove(id2); err != nil {
		t.Fatalf("failed to remove id2: %v", err)
	}

	// Verify list is empty
	head, err = linkedList.Peek()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())
}

func Test_LinkedList_Iter(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	// Create 3 addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	// Add addresses to the linked list
	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	if err := linkedList.Add(id3); err != nil {
		t.Fatalf("failed to add id3: %v", err)
	}

	// Test iteration
	var addresses []thor.Address
	count := 0

	err := linkedList.Iter(func(address thor.Address) error {
		addresses = append(addresses, address)
		count++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, len(addresses))
	assert.Equal(t, id1, addresses[0])
	assert.Equal(t, id2, addresses[1])
	assert.Equal(t, id3, addresses[2])
	assert.Equal(t, 3, count)

	// Test early termination
	addresses = []thor.Address{}
	err = linkedList.Iter(func(address thor.Address) error {
		addresses = append(addresses, address)
		if len(addresses) >= 2 {
			return errors.New("early termination")
		}
		return nil
	})

	assert.Error(t, err)
	assert.Equal(t, "early termination", err.Error())
	assert.Equal(t, 2, len(addresses))
	assert.Equal(t, id1, addresses[0])
	assert.Equal(t, id2, addresses[1])

	// Test iteration on empty list
	emptyList := NewLinkedList(sctx, thor.Bytes32{0x4}, thor.Bytes32{0x5}, thor.Bytes32{0x6})
	var emptyResult []thor.Address

	err = emptyList.Iter(func(address thor.Address) error {
		emptyResult = append(emptyResult, address)
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 0, len(emptyResult))
}

func Test_LinkedList_Pop(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	// Test pop on empty list
	_, err := linkedList.Pop()
	assert.Error(t, err)
	assert.Equal(t, "list is empty", err.Error())

	// Add some addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	// Pop first element
	popped, err := linkedList.Pop()
	assert.NoError(t, err)
	assert.Equal(t, id1, popped)

	// Verify head is now id2
	head, err := linkedList.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id2, head)

	// Pop second element
	popped, err = linkedList.Pop()
	assert.NoError(t, err)
	assert.Equal(t, id2, popped)

	// Verify list is empty
	head, err = linkedList.Peek()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())
}

func Test_LinkedList_Len(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	// Test empty list
	len, err := linkedList.Len()
	assert.NoError(t, err)
	assert.Zero(t, big.NewInt(0).Cmp(len))

	// Add addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	len, err = linkedList.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), len)

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	len, err = linkedList.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), len)

	// Remove one address
	if err := linkedList.Remove(id1); err != nil {
		t.Fatalf("failed to remove id1: %v", err)
	}

	len, err = linkedList.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), len)
}

func Test_LinkedList_Next(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	linkedList := NewLinkedList(sctx, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	// Test Next on empty list
	next, err := linkedList.Next(datagen.RandAddress())
	assert.NoError(t, err)
	assert.True(t, next.IsZero())

	// Add addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	if err := linkedList.Add(id1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := linkedList.Add(id2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	if err := linkedList.Add(id3); err != nil {
		t.Fatalf("failed to add id3: %v", err)
	}

	// Test Next for each address
	next, err = linkedList.Next(id1)
	assert.NoError(t, err)
	assert.Equal(t, id2, next)

	next, err = linkedList.Next(id2)
	assert.NoError(t, err)
	assert.Equal(t, id3, next)

	next, err = linkedList.Next(id3)
	assert.NoError(t, err)
	assert.True(t, next.IsZero()) // id3 is the last element
}

func Test_LinkedList_Grow_Empty_Grow(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	owner := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(owner, st, nil)

	ll := NewLinkedList(sctx,
		thor.Bytes32{0x1}, // head
		thor.Bytes32{0x2}, // tail
		thor.Bytes32{0x3}, // count
	)

	// --- 1) Grow first time
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	assert.NoError(t, ll.Add(id1))
	assert.NoError(t, ll.Add(id2))

	// head should be id1
	head, err := ll.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// next(id1) == id2, next(id2).IsZero()
	nxt, err := ll.Next(id1)
	assert.NoError(t, err)
	assert.Equal(t, id2, nxt)
	nxt, err = ll.Next(id2)
	assert.NoError(t, err)
	assert.True(t, nxt.IsZero())

	// length == 2
	ln, err := ll.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), ln)

	// --- 2) Drain to zero with Pop
	popped, err := ll.Pop()
	assert.NoError(t, err)
	assert.Equal(t, id1, popped)
	popped, err = ll.Pop()
	assert.NoError(t, err)
	assert.Equal(t, id2, popped)

	head, err = ll.Peek()
	assert.NoError(t, err)
	assert.True(t, head.IsZero(), "expected empty head after draining")

	ln, err = ll.Len()
	assert.NoError(t, err)
	assert.Zero(t, big.NewInt(0).Cmp(ln))

	// --- 3) Grow second time
	id3 := datagen.RandAddress()
	id4 := datagen.RandAddress()

	assert.NoError(t, ll.Add(id3))
	assert.NoError(t, ll.Add(id4))

	// head should reset to id3
	head, err = ll.Peek()
	assert.NoError(t, err)
	assert.Equal(t, id3, head)

	// pointers should be fresh
	nxt, err = ll.Next(id3)
	assert.NoError(t, err)
	assert.Equal(t, id4, nxt)
	nxt, err = ll.Next(id4)
	assert.NoError(t, err)
	assert.True(t, nxt.IsZero())

	// length should now be 2 again
	ln, err = ll.Len()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), ln)
}
