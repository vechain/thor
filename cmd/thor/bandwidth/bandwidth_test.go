// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bandwidth

import (
	"crypto/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

func TestBandwidth(t *testing.T) {
	bandwidth := Bandwidth{
		value: 0,
		lock:  sync.Mutex{},
	}

	val := bandwidth.Value()

	assert.Equal(t, uint64(0), val)
}

func GetMockHeader(t *testing.T) *block.Header {
	var sig [65]byte
	rand.Read(sig[:])

	block := new(block.Builder).Build().WithSignature(sig[:])
	h := block.Header()
	return h
}

func TestBandwithUpdate(t *testing.T) {
	bandwidth := Bandwidth{
		value: 0,
		lock:  sync.Mutex{},
	}

	block := new(
		block.Builder,
	).ParentID(thor.Bytes32{1}).
		Timestamp(1).
		GasLimit(100000).
		Beneficiary(thor.Address{1}).
		GasUsed(11234).
		TotalScore(1).
		StateRoot(thor.Bytes32{1}).
		ReceiptsRoot(thor.Bytes32{1}).
		Build()
	header := block.Header()

	bandwidth.Update(header, 1)
	val := bandwidth.Value()

	assert.Equal(t, uint64(11234000000000), val)
}

func TestBandwidthSuggestGasLimit(t *testing.T) {
	bandwidth := Bandwidth{
		value: 0,
		lock:  sync.Mutex{},
	}

	block := new(
		block.Builder,
	).ParentID(thor.Bytes32{1}).
		Timestamp(1).
		GasLimit(100000).
		Beneficiary(thor.Address{1}).
		GasUsed(11234).
		TotalScore(1).
		StateRoot(thor.Bytes32{1}).
		ReceiptsRoot(thor.Bytes32{1}).
		Build()
	header := block.Header()
	bandwidth.Update(header, 1)
	val := bandwidth.SuggestGasLimit()

	assert.Equal(t, uint64(5617000000000), val)
}
