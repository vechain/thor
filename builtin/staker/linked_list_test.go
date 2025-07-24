// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

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
	linkedList := newLinkedList(newStorage(addr, st, nil), thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	validator1ID := datagen.RandomHash()
	validator1 := &Validation{
		Node: datagen.RandAddress(),
	}

	validator2ID := datagen.RandomHash()
	validator2 := &Validation{
		Node: datagen.RandAddress(),
	}

	validator3ID := datagen.RandomHash()
	validator3 := &Validation{
		Node: datagen.RandAddress(),
	}

	if _, err := linkedList.Add(validator1ID, validator1); err != nil {
		t.Fatalf("failed to add validator 1: %v", err)
	}

	if _, err := linkedList.Add(validator2ID, validator2); err != nil {
		t.Fatalf("failed to add validator 2: %v", err)
	}

	if _, err := linkedList.Remove(validator3ID, validator3); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	head, err := linkedList.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, validator1ID, head)
}

func Test_LinkedList_Remove(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	storage := newStorage(addr, st, nil)
	linkedList := newLinkedList(storage, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	validator1ID := datagen.RandomHash()
	validator1 := &Validation{
		Node:      datagen.RandAddress(),
		Endorsor:  datagen.RandAddress(),
		LockedVET: big.NewInt(10),
		Weight:    big.NewInt(10),
		Status:    2,
	}

	validator2ID := datagen.RandomHash()
	validator2 := &Validation{
		Node:      datagen.RandAddress(),
		Endorsor:  datagen.RandAddress(),
		LockedVET: big.NewInt(10),
		Weight:    big.NewInt(10),
		Status:    2,
	}

	if _, err := linkedList.Add(validator1ID, validator1); err != nil {
		t.Fatalf("failed to add validator 1: %v", err)
	}

	if _, err := linkedList.Add(validator2ID, validator2); err != nil {
		t.Fatalf("failed to add validator 2: %v", err)
	}

	head, err := linkedList.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, validator1ID, head)
	validator1, err = storage.GetValidation(validator1ID)
	assert.NoError(t, err)

	if _, err := linkedList.Remove(validator1ID, validator1); err != nil {
		t.Fatalf("failed to remove validator 1: %v", err)
	}

	head, err = linkedList.head.Get()
	assert.NoError(t, err)
	assert.Equal(t, validator2ID, head)
	validator, err := storage.GetValidation(validator1ID)
	assert.NoError(t, err)
	assert.Nil(t, validator.Next)
	assert.Nil(t, validator.Prev)

	if _, err := linkedList.Remove(validator2ID, validator1); err != nil {
		t.Fatalf("failed to remove validator 2: %v", err)
	}
	head, err = linkedList.head.Get()
	assert.NoError(t, err)
	assert.True(t, head.IsZero())
	validator, err = storage.GetValidation(validator2ID)
	assert.NoError(t, err)
	assert.Nil(t, validator.Next)
	assert.Nil(t, validator.Prev)
}

func Test_LinkedList_Iter(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	storage := newStorage(addr, st, nil)
	linkedList := newLinkedList(storage, thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	// Create 3 validators
	validator1ID := datagen.RandomHash()
	validator1 := &Validation{
		Node:      datagen.RandAddress(),
		Endorsor:  datagen.RandAddress(),
		LockedVET: big.NewInt(10),
		Weight:    big.NewInt(10),
		Status:    2,
	}

	validator2ID := datagen.RandomHash()
	validator2 := &Validation{
		Node:      datagen.RandAddress(),
		Endorsor:  datagen.RandAddress(),
		LockedVET: big.NewInt(20),
		Weight:    big.NewInt(20),
		Status:    2,
	}

	validator3ID := datagen.RandomHash()
	validator3 := &Validation{
		Node:      datagen.RandAddress(),
		Endorsor:  datagen.RandAddress(),
		LockedVET: big.NewInt(30),
		Weight:    big.NewInt(30),
		Status:    2,
	}

	// Add validators to the linked list
	if _, err := linkedList.Add(validator1ID, validator1); err != nil {
		t.Fatalf("failed to add validator 1: %v", err)
	}

	if _, err := linkedList.Add(validator2ID, validator2); err != nil {
		t.Fatalf("failed to add validator 2: %v", err)
	}

	if _, err := linkedList.Add(validator3ID, validator3); err != nil {
		t.Fatalf("failed to add validator 3: %v", err)
	}

	// Test iteration
	var validatorIDs []thor.Bytes32
	totalWeight := big.NewInt(0)

	err := linkedList.Iter(func(id thor.Bytes32, validator *Validation) error {
		validatorIDs = append(validatorIDs, id)
		totalWeight = totalWeight.Add(totalWeight, validator.Weight)
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, len(validatorIDs))
	assert.Equal(t, validator1ID, validatorIDs[0])
	assert.Equal(t, validator2ID, validatorIDs[1])
	assert.Equal(t, validator3ID, validatorIDs[2])
	assert.Equal(t, big.NewInt(60), totalWeight)

	// Test early termination
	validatorIDs = []thor.Bytes32{}
	err = linkedList.Iter(func(id thor.Bytes32, validator *Validation) error {
		validatorIDs = append(validatorIDs, id)
		if len(validatorIDs) >= 2 {
			return errors.New("early termination")
		}
		return nil
	})

	assert.Error(t, err)
	assert.Equal(t, "early termination", err.Error())
	assert.Equal(t, 2, len(validatorIDs))
	assert.Equal(t, validator1ID, validatorIDs[0])
	assert.Equal(t, validator2ID, validatorIDs[1])

	// Test iteration on empty list
	emptyList := newLinkedList(storage, thor.Bytes32{0x4}, thor.Bytes32{0x5}, thor.Bytes32{0x6})
	var emptyResult []thor.Bytes32

	err = emptyList.Iter(func(id thor.Bytes32, validator *Validation) error {
		emptyResult = append(emptyResult, id)
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 0, len(emptyResult))
}
