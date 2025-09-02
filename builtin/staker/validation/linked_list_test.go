//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func Test_LinkedList_HeadAndTail(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	head, err := repo.queuedList.GetHead()
	assert.NoError(t, err)
	assert.NotNil(t, head)
	assert.Equal(t, id1, *head)

	tail, err := repo.queuedList.GetTail()
	assert.NoError(t, err)
	assert.NotNil(t, tail)
	assert.Equal(t, id1, *tail)

	val, err := repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	assert.Nil(t, val.Prev)
	assert.Nil(t, val.Next)

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	head, err = repo.queuedList.GetHead()
	assert.NoError(t, err)
	assert.NotNil(t, head)
	assert.Equal(t, id1, *head)

	tail, err = repo.queuedList.GetTail()
	assert.NoError(t, err)
	assert.NotNil(t, tail)
	assert.Equal(t, id2, *tail)

	cnt, err := repo.queuedList.GetSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), cnt)

	val, err = repo.getValidation(id2)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	assert.NotNil(t, val.Prev)
	assert.Nil(t, val.Next)
	assert.Equal(t, id1, *val.Prev)

	val, err = repo.getValidation(id2)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())
	// remove ID2
	if err := repo.removeQueued(id2, val); err != nil {
		t.Fatalf("failed to remove id2: %v", err)
	}

	cnt, err = repo.queuedList.GetSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), cnt)

	head, err = repo.queuedList.GetHead()
	assert.NoError(t, err)
	assert.NotNil(t, head)
	assert.Equal(t, id1, *head)

	tail, err = repo.queuedList.GetTail()
	assert.NoError(t, err)
	assert.NotNil(t, tail)
	assert.Equal(t, id1, *tail)

	val, err = repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	// remove ID1
	if err := repo.removeQueued(id1, val); err != nil {
		t.Fatalf("failed to remove id1: %v", err)
	}

	cnt, err = repo.queuedList.GetSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), cnt)

	head, err = repo.queuedList.GetHead()
	assert.NoError(t, err)
	assert.Nil(t, head)

	tail, err = repo.queuedList.GetTail()
	assert.NoError(t, err)
	assert.Nil(t, tail)
}

func Test_LinkedList_Remove_NonExistent(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	head, err := repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// Try to remove non-existent id
	if err := repo.removeQueued(id3, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("expected no error when removing non-existent id, got: %v", err)
	}

	//Verify head is still id1
	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// Verify removed-node pointers are cleared
	nextPtr, err := repo.nextEntry(id2)
	assert.NoError(t, err)
	assert.True(t, nextPtr.IsZero(), "expected next[id2] to be cleared")
	prevPtr, err := repo.getValidation(id1)
	assert.NoError(t, err)
	assert.Nil(t, prevPtr.Prev, "expected prev[id1] to be cleared")

	// Try to remove non-existent id
	if err := repo.removeQueued(id3, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("expected no error when removing non-existent id, got: %v", err)
	}

	// Head unchanged
	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// Tail unchanged (i.e. Next(id2) is zero)
	next, err := repo.nextEntry(id2)
	assert.NoError(t, err)
	assert.True(t, next.IsZero())

	// Length unchanged
	length, err := repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), length)

	// Link from id1 → id2 still intact
	next, err = repo.nextEntry(id1)
	assert.NoError(t, err)
	assert.Equal(t, id2, next)
}

func Test_LinkedList_Remove_NegativeTests(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))

	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	id1 := thor.Address{}
	id2 := thor.Address{}

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	assert.Nil(t, repo.removeQueued(thor.Address{}, &Validation{Status: StatusQueued}))
}

func Test_LinkedList_Remove(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	// Verify head is id1
	head, err := repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	val, err := repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	// Remove id1
	if err := repo.removeQueued(id1, val); err != nil {
		t.Fatalf("failed to remove id1: %v", err)
	}

	// Verify head is now id2
	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id2, head)

	val, err = repo.getValidation(id2)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	// Remove id2
	if err := repo.removeQueued(id2, val); err != nil {
		t.Fatalf("failed to remove id2: %v", err)
	}

	// Verify list is empty
	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())
}

func Test_LinkedList_Iter(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	// Create 3 addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	// Add addresses to the linked list
	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	val, err := repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	if err := repo.addActive(id1, val); err != nil {
		t.Fatalf("failed to activate id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	val, err = repo.getValidation(id2)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	if err := repo.addActive(id2, val); err != nil {
		t.Fatalf("failed to activate id2: %v", err)
	}

	if err := repo.addValidation(id3, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id3: %v", err)
	}

	val, err = repo.getValidation(id3)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	if err := repo.addActive(id3, val); err != nil {
		t.Fatalf("failed to activate id3: %v", err)
	}

	// Test iteration
	var addresses []thor.Address
	count := 0

	err = repo.iterateActive(func(address thor.Address, entry *Validation) error {
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
	err = repo.iterateActive(func(address thor.Address, entry *Validation) error {
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

	db = muxdb.NewMem()
	st = state.New(db, trie.Root{})
	addr = thor.BytesToAddress([]byte("test"))
	sctx = solidity.NewContext(addr, st, nil)
	// Test iteration on empty list
	emptyList := NewRepository(sctx)
	var emptyResult []thor.Address

	err = emptyList.iterateActive(func(address thor.Address, entry *Validation) error {
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

	repo := NewRepository(sctx)

	// Test pop on empty list
	_, _, err := repo.popQueued()
	assert.Error(t, err)
	assert.Equal(t, "list is empty", err.Error())

	// Add some addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	// Pop first element
	popped, _, err := repo.popQueued()
	assert.NoError(t, err)
	assert.Equal(t, id1, popped)

	// Verify head is now id2
	head, err := repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id2, head)

	// Pop second element
	popped, _, err = repo.popQueued()
	assert.NoError(t, err)
	assert.Equal(t, id2, popped)

	// Verify list is empty
	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())
}

func Test_LinkedList_Len(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	// Test empty list
	len, err := repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), len)

	// Add addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	len, err = repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), len)

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	len, err = repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), len)

	val, err := repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val.IsEmpty())

	// Remove one address
	if err := repo.removeQueued(id1, val); err != nil {
		t.Fatalf("failed to remove id1: %v", err)
	}

	len, err = repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), len)
}

func Test_LinkedList_Next(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	// Test Next on empty list
	next, err := repo.nextEntry(datagen.RandAddress())
	assert.NoError(t, err)
	assert.True(t, next.IsZero())

	// Add addresses
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	if err := repo.addValidation(id3, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id3: %v", err)
	}

	// Test Next for each address
	next, err = repo.nextEntry(id1)
	assert.NoError(t, err)
	assert.Equal(t, id2, next)

	next, err = repo.nextEntry(id2)
	assert.NoError(t, err)
	assert.Equal(t, id3, next)

	next, err = repo.nextEntry(id3)
	assert.NoError(t, err)
	assert.True(t, next.IsZero()) // id3 is the last element
}

func Test_LinkedList_Grow_Empty_Grow(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	owner := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(owner, st, nil)

	repo := NewRepository(sctx)

	// --- 1) Grow first time
	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()

	assert.NoError(t, repo.addValidation(id1, &Validation{Status: StatusQueued}))
	assert.NoError(t, repo.addValidation(id2, &Validation{Status: StatusQueued}))

	// head should be id1
	head, err := repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id1, head)

	// next(id1) == id2, next(id2).IsZero()
	nxt, err := repo.nextEntry(id1)
	assert.NoError(t, err)
	assert.Equal(t, id2, nxt)
	nxt, err = repo.nextEntry(id2)
	assert.NoError(t, err)
	assert.True(t, nxt.IsZero())

	// length == 2
	ln, err := repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), ln)

	// --- 2) Drain to zero with Pop
	popped, _, err := repo.popQueued()
	assert.NoError(t, err)
	assert.Equal(t, id1, popped)
	popped, _, err = repo.popQueued()
	assert.NoError(t, err)
	assert.Equal(t, id2, popped)

	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.True(t, head.IsZero(), "expected empty head after draining")

	ln, err = repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), ln)

	// --- 3) Grow second time
	id3 := datagen.RandAddress()
	id4 := datagen.RandAddress()

	assert.NoError(t, repo.addValidation(id3, &Validation{Status: StatusQueued}))
	assert.NoError(t, repo.addValidation(id4, &Validation{Status: StatusQueued}))

	// head should reset to id3
	head, err = repo.queuedListHead()
	assert.NoError(t, err)
	assert.Equal(t, id3, head)

	// pointers should be fresh
	nxt, err = repo.nextEntry(id3)
	assert.NoError(t, err)
	assert.Equal(t, id4, nxt)
	nxt, err = repo.nextEntry(id4)
	assert.NoError(t, err)
	assert.True(t, nxt.IsZero())

	// length should now be 2 again
	ln, err = repo.queuedListSize()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), ln)
}

func Test_LinkedList_Iter_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	sctx := solidity.NewContext(addr, st, nil)

	repo := NewRepository(sctx)

	id1 := datagen.RandAddress()
	id2 := datagen.RandAddress()
	id3 := datagen.RandAddress()

	if err := repo.addValidation(id1, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	val1, err := repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val1.IsEmpty())

	if err := repo.addActive(id1, val1); err != nil {
		t.Fatalf("failed to add id1: %v", err)
	}

	if err := repo.addValidation(id2, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	val2, err := repo.getValidation(id2)
	assert.NoError(t, err)
	assert.False(t, val1.IsEmpty())

	if err := repo.addActive(id2, val2); err != nil {
		t.Fatalf("failed to add id2: %v", err)
	}

	if err := repo.addValidation(id3, &Validation{Status: StatusQueued}); err != nil {
		t.Fatalf("failed to add id3: %v", err)
	}

	val3, err := repo.getValidation(id3)
	assert.NoError(t, err)
	assert.False(t, val1.IsEmpty())

	if err := repo.addActive(id3, val3); err != nil {
		t.Fatalf("failed to add id3: %v", err)
	}

	var addresses []thor.Address
	count := 0

	raw, err := st.GetRawStorage(addr, slotActiveHead)
	assert.NoError(t, err)
	st.SetRawStorage(addr, slotActiveHead, rlp.RawValue{0xFF})
	err = repo.iterateActive(func(address thor.Address, entry *Validation) error {
		addresses = append(addresses, address)
		count++
		return nil
	})
	assert.ErrorContains(t, err, "state: rlp")

	st.SetRawStorage(addr, slotActiveHead, raw)
	slot := thor.Blake2b(id1.Bytes(), slotValidations.Bytes())
	raw, err = st.GetRawStorage(addr, slot)
	assert.NoError(t, err)
	st.SetRawStorage(addr, slot, rlp.RawValue{0xFF})
	err = repo.iterateActive(func(address thor.Address, entry *Validation) error {
		addresses = append(addresses, address)
		count++
		return nil
	})
	assert.ErrorContains(t, err, "state: rlp: value size exceeds available input length")

	st.SetRawStorage(addr, slot, raw)
	raw, err = st.GetRawStorage(addr, slotActiveTail)
	assert.NoError(t, err)
	st.SetRawStorage(addr, slotActiveTail, rlp.RawValue{0xFF})
	err = repo.removeActive(id1, val1)
	assert.ErrorContains(t, err, "state: rlp")

	st.SetRawStorage(addr, slotActiveTail, raw)
	st.SetRawStorage(addr, slotActiveGroupSize, rlp.RawValue{0xFF})
	val1, err = repo.getValidation(id1)
	assert.NoError(t, err)
	assert.False(t, val1.IsEmpty())
	err = repo.removeActive(id1, val1)
	assert.ErrorContains(t, err, "state: rlp")
}
