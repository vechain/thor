// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/solidity"
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

	// Create validators mapping
	validators := solidity.NewMapping[thor.Address, *Validator](addr, st, thor.Bytes32{})

	// Create ordered linked list
	list := newOrderedLinkedList(addr, st, validators, headPos, tailPos)

	// Test cases
	tests := []struct {
		name       string
		validators []struct {
			addr   thor.Address
			weight uint64
		}
		expectedOrder []thor.Address
	}{
		{
			name: "empty list",
			validators: []struct {
				addr   thor.Address
				weight uint64
			}{
				{thor.BytesToAddress([]byte("v1")), 100},
			},
			expectedOrder: []thor.Address{
				thor.BytesToAddress([]byte("v1")),
			},
		},
		{
			name: "insert at head",
			validators: []struct {
				addr   thor.Address
				weight uint64
			}{
				{thor.BytesToAddress([]byte("v1")), 100},
				{thor.BytesToAddress([]byte("v2")), 200},
				{thor.BytesToAddress([]byte("v3")), 300},
			},
			expectedOrder: []thor.Address{
				thor.BytesToAddress([]byte("v3")),
				thor.BytesToAddress([]byte("v2")),
				thor.BytesToAddress([]byte("v1")),
			},
		},
		{
			name: "insert at tail",
			validators: []struct {
				addr   thor.Address
				weight uint64
			}{
				{thor.BytesToAddress([]byte("v3")), 300},
				{thor.BytesToAddress([]byte("v2")), 200},
				{thor.BytesToAddress([]byte("v1")), 100},
			},
			expectedOrder: []thor.Address{
				thor.BytesToAddress([]byte("v3")),
				thor.BytesToAddress([]byte("v2")),
				thor.BytesToAddress([]byte("v1")),
			},
		},
		{
			name: "insert in middle",
			validators: []struct {
				addr   thor.Address
				weight uint64
			}{
				{thor.BytesToAddress([]byte("v3")), 300},
				{thor.BytesToAddress([]byte("v1")), 100},
				{thor.BytesToAddress([]byte("v2")), 200},
				{thor.BytesToAddress([]byte("v4")), 400},
				{thor.BytesToAddress([]byte("v22")), 200},
				{thor.BytesToAddress([]byte("v15")), 150},
				{thor.BytesToAddress([]byte("v5")), 500},
				{thor.BytesToAddress([]byte("v6")), 600},
			},
			expectedOrder: []thor.Address{
				thor.BytesToAddress([]byte("v6")),
				thor.BytesToAddress([]byte("v5")),
				thor.BytesToAddress([]byte("v4")),
				thor.BytesToAddress([]byte("v3")),
				thor.BytesToAddress([]byte("v2")),
				thor.BytesToAddress([]byte("v22")),
				thor.BytesToAddress([]byte("v15")),
				thor.BytesToAddress([]byte("v1")),
			},
		},
		{
			name: "equal weights",
			validators: []struct {
				addr   thor.Address
				weight uint64
			}{
				{thor.BytesToAddress([]byte("v1")), 100},
				{thor.BytesToAddress([]byte("v2")), 100},
				{thor.BytesToAddress([]byte("v3")), 100},
			},
			expectedOrder: []thor.Address{
				thor.BytesToAddress([]byte("v1")),
				thor.BytesToAddress([]byte("v2")),
				thor.BytesToAddress([]byte("v3")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state for each test
			st = state.New(db, trie.Root{})
			validators = solidity.NewMapping[thor.Address, *Validator](addr, st, thor.Bytes32{})
			list = newOrderedLinkedList(addr, st, validators, headPos, tailPos)

			// Add validators
			for _, v := range tt.validators {
				validator := &Validator{
					Weight: big.NewInt(int64(v.weight)),
				}
				err := list.Add(v.addr, validator)
				assert.NoError(t, err)
			}

			// Verify order by popping all validators
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
