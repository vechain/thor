// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
)

func TestBeat2Reader_Read(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)

	genesisBlk := allBlocks[0]
	newBlock := allBlocks[1]

	// Act
	beatReader := newBeat2Reader(thorChain.Repo(), genesisBlk.Header().ID(), newMessageCache[Beat2Message](10))
	res, ok, err := beatReader.Read()

	// Assert
	assert.NoError(t, err)
	assert.True(t, ok)
	if beatMsg, ok := res[0].(Beat2Message); !ok {
		t.Fatal("unexpected type")
	} else {
		assert.Equal(t, newBlock.Header().Number(), beatMsg.Number)
		assert.Equal(t, newBlock.Header().ID(), beatMsg.ID)
		assert.Equal(t, newBlock.Header().ParentID(), beatMsg.ParentID)
		assert.Equal(t, newBlock.Header().Timestamp(), beatMsg.Timestamp)
		assert.Equal(t, uint32(newBlock.Header().TxsFeatures()), beatMsg.TxsFeatures)
		// GasLimit is not part of the deprecated BeatMessage
		assert.Equal(t, newBlock.Header().GasLimit(), beatMsg.GasLimit)
	}
}

func TestBeat2Reader_Read_NoNewBlocksToRead(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	newBlock := allBlocks[1]

	// Act
	beatReader := newBeat2Reader(thorChain.Repo(), newBlock.Header().ID(), newMessageCache[Beat2Message](10))
	res, ok, err := beatReader.Read()

	// Assert
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func TestBeat2Reader_Read_ErrorWhenReadingBlocks(t *testing.T) {
	// Arrange
	thorChain := initChain(t)

	// Act
	beatReader := newBeat2Reader(thorChain.Repo(), thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), newMessageCache[Beat2Message](10))
	res, ok, err := beatReader.Read()

	// Assert
	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}
