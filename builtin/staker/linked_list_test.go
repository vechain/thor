// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
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
	linkedList := newLinkedList(newStorage(addr, st), thor.Bytes32{0x1}, thor.Bytes32{0x2}, thor.Bytes32{0x3})

	validator1ID := datagen.RandomHash()
	validator1 := &Validation{
		Master: datagen.RandAddress(),
	}

	validator2ID := datagen.RandomHash()
	validator2 := &Validation{
		Master: datagen.RandAddress(),
	}

	validator3ID := datagen.RandomHash()
	validator3 := &Validation{
		Master: datagen.RandAddress(),
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
