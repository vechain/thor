// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestBeatReader_Read(t *testing.T) {
	// Arrange
	repo, generatedBlocks, _ := initChain(t)
	genesisBlk := generatedBlocks[0]
	newBlock := generatedBlocks[1]

	// Act
	beatReader := newBeatReader(repo, genesisBlk.Header().ID(), newMessageCache[BeatMessage](10))
	res, ok, err := beatReader.Read()

	// Assert
	assert.NoError(t, err)
	assert.True(t, ok)
	beat, ok := res[0].(BeatMessage)
	assert.True(t, ok)
	assert.NoError(t, err)

	assert.Equal(t, newBlock.Header().Number(), beat.Number)
	assert.Equal(t, newBlock.Header().ID(), beat.ID)
	assert.Equal(t, newBlock.Header().ParentID(), beat.ParentID)
	assert.Equal(t, newBlock.Header().Timestamp(), beat.Timestamp)
	assert.Equal(t, uint32(newBlock.Header().TxsFeatures()), beat.TxsFeatures)
}

func TestBeatReader_Read_NoNewBlocksToRead(t *testing.T) {
	// Arrange
	repo, generatedBlocks, _ := initChain(t)
	newBlock := generatedBlocks[1]

	// Act
	beatReader := newBeatReader(repo, newBlock.Header().ID(), newMessageCache[BeatMessage](10))
	res, ok, err := beatReader.Read()

	// Assert
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func TestBeatReader_Read_ErrorWhenReadingBlocks(t *testing.T) {
	// Arrange
	repo, _, _ := initChain(t)

	// Act
	beatReader := newBeatReader(repo, thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), newMessageCache[BeatMessage](10))
	res, ok, err := beatReader.Read()

	// Assert
	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}
