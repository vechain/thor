// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

func TestConvertEvent(t *testing.T) {
	event := &logdb.Event{
		Address:     thor.Address{0x01},
		Data:        []byte{0x02, 0x03},
		BlockID:     thor.Bytes32{0x04},
		BlockNumber: 5,
		BlockTime:   6,
		TxID:        thor.Bytes32{0x07},
		TxIndex:     8,
		LogIndex:    9,
		TxOrigin:    thor.Address{0x0A},
		ClauseIndex: 10,
		Topics: [5]*thor.Bytes32{
			{0x0B},
			{0x0C},
			nil,
			nil,
			nil,
		},
	}

	expectedTopics := []*thor.Bytes32{
		{0x0B},
		{0x0C},
	}
	expectedData := hexutil.Encode(event.Data)

	result := ConvertEvent(event, true)

	assert.Equal(t, event.Address, result.Address)
	assert.Equal(t, expectedData, result.Data)
	assert.Equal(t, event.BlockID, result.Meta.BlockID)
	assert.Equal(t, event.BlockNumber, result.Meta.BlockNumber)
	assert.Equal(t, event.BlockTime, result.Meta.BlockTimestamp)
	assert.Equal(t, event.TxID, result.Meta.TxID)
	assert.Equal(t, event.TxIndex, *result.Meta.TxIndex)
	assert.Equal(t, event.LogIndex, *result.Meta.LogIndex)
	assert.Equal(t, event.TxOrigin, result.Meta.TxOrigin)
	assert.Equal(t, event.ClauseIndex, result.Meta.ClauseIndex)
	assert.Equal(t, expectedTopics, result.Topics)
}
