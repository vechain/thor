// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestOrderedLinkedList_Add(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("test"))
	headPos := thor.Bytes32{}
	tailPos := thor.Bytes32{1}

	// Create ordered linked list
	list := newOrderedLinkedList(newStorage(addr, st), headPos, tailPos)

	// Test cases
	tests := []struct {
		name       string
		validators []struct {
			id    thor.Bytes32
			stake uint64
		}
		expectedOrder []thor.Bytes32
	}{
		{
			name: "empty list",
			validators: []struct {
				id    thor.Bytes32
				stake uint64
			}{
				{thor.BytesToBytes32([]byte("v1")), 100},
			},
			expectedOrder: []thor.Bytes32{
				thor.BytesToBytes32([]byte("v1")),
			},
		},
		{
			name: "insert at head",
			validators: []struct {
				id    thor.Bytes32
				stake uint64
			}{
				{thor.BytesToBytes32([]byte("v1")), 100},
				{thor.BytesToBytes32([]byte("v2")), 200},
				{thor.BytesToBytes32([]byte("v3")), 300},
			},
			expectedOrder: []thor.Bytes32{
				thor.BytesToBytes32([]byte("v3")),
				thor.BytesToBytes32([]byte("v2")),
				thor.BytesToBytes32([]byte("v1")),
			},
		},
		{
			name: "insert at tail",
			validators: []struct {
				id    thor.Bytes32
				stake uint64
			}{
				{thor.BytesToBytes32([]byte("v3")), 300},
				{thor.BytesToBytes32([]byte("v2")), 200},
				{thor.BytesToBytes32([]byte("v1")), 100},
			},
			expectedOrder: []thor.Bytes32{
				thor.BytesToBytes32([]byte("v3")),
				thor.BytesToBytes32([]byte("v2")),
				thor.BytesToBytes32([]byte("v1")),
			},
		},
		{
			name: "insert in middle",
			validators: []struct {
				id    thor.Bytes32
				stake uint64
			}{
				{thor.BytesToBytes32([]byte("v3")), 300},
				{thor.BytesToBytes32([]byte("v1")), 100},
				{thor.BytesToBytes32([]byte("v2")), 200},
				{thor.BytesToBytes32([]byte("v4")), 400},
				{thor.BytesToBytes32([]byte("v22")), 200},
				{thor.BytesToBytes32([]byte("v15")), 150},
				{thor.BytesToBytes32([]byte("v5")), 500},
				{thor.BytesToBytes32([]byte("v6")), 600},
			},
			expectedOrder: []thor.Bytes32{
				thor.BytesToBytes32([]byte("v6")),
				thor.BytesToBytes32([]byte("v5")),
				thor.BytesToBytes32([]byte("v4")),
				thor.BytesToBytes32([]byte("v3")),
				thor.BytesToBytes32([]byte("v2")),
				thor.BytesToBytes32([]byte("v22")),
				thor.BytesToBytes32([]byte("v15")),
				thor.BytesToBytes32([]byte("v1")),
			},
		},
		{
			name: "equal weights",
			validators: []struct {
				id    thor.Bytes32
				stake uint64
			}{
				{thor.BytesToBytes32([]byte("v1")), 100},
				{thor.BytesToBytes32([]byte("v2")), 100},
				{thor.BytesToBytes32([]byte("v3")), 100},
			},
			expectedOrder: []thor.Bytes32{
				thor.BytesToBytes32([]byte("v1")),
				thor.BytesToBytes32([]byte("v2")),
				thor.BytesToBytes32([]byte("v3")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state for each test
			st = state.New(db, trie.Root{})
			list = newOrderedLinkedList(newStorage(addr, st), headPos, tailPos)

			// Add validations
			for _, v := range tt.validators {
				validator := &Validation{
					PendingLocked: big.NewInt(int64(v.stake)),
				}
				err := list.Add(v.id, validator)
				assert.NoError(t, err)
			}

			// Verify order by popping all validations
			for _, expectedAddr := range tt.expectedOrder {
				address, validator, err := list.Pop()
				assert.NoError(t, err)
				assert.Equal(t, expectedAddr, address)
				assert.NotNil(t, validator)
			}

			// Verify list is empty
			_, _, err := list.Pop()
			assert.Error(t, err, "no head present")
		})
	}
}
