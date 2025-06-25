// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/types"
	"github.com/vechain/thor/v2/chain"
)

func TestEventReader_Read(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	genesisBlk := allBlocks[0]
	// taking best block to include also galactica block
	bestBlk := allBlocks[len(allBlocks)-1]

	er := &eventReader{
		repo:        thorChain.Repo(),
		filter:      &types.SubscriptionEventFilter{},
		blockReader: &mockBlockReaderWithError{},
	}

	// Test case 1: An error occurred while reading blocks
	events, ok, err := er.Read()
	assert.Error(t, err)
	assert.Empty(t, events)
	assert.False(t, ok)

	// Test case 2: There are no events available to read
	er = newEventReader(thorChain.Repo(), genesisBlk.Header().ID(), &types.SubscriptionEventFilter{})

	events, ok, err = er.Read()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Zero(t, len(events))

	// Test case 3: There are no events available to read
	events, ok, err = er.Read()
	assert.NoError(t, err)
	assert.True(t, ok)

	var eventMessages []*types.EventMessage
	for _, event := range events {
		if msg, ok := event.(*types.EventMessage); ok {
			eventMessages = append(eventMessages, msg)
		} else {
			t.Fatal("unexpected type")
		}
	}
	assert.Equal(t, 2, len(eventMessages))
	eventMsg := eventMessages[0]
	assert.Equal(t, bestBlk.Header().ID(), eventMsg.Meta.BlockID)
	assert.Equal(t, bestBlk.Header().Number(), eventMsg.Meta.BlockNumber)
}

type mockBlockReaderWithError struct{}

func (m *mockBlockReaderWithError) Read() ([]*chain.ExtendedBlock, error) {
	return nil, assert.AnError
}
