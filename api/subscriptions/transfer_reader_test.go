// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestTransferReader_Read(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	genesisBlk := allBlocks[0]
	newBlock := allBlocks[1]
	filter := &TransferFilter{}

	// Act
	br := newTransferReader(thorChain.Repo(), genesisBlk.Header().ID(), filter)
	res, ok, err := br.Read()

	// Assert
	assert.NoError(t, err)
	assert.True(t, ok)
	if transferMsg, ok := res[0].(*TransferMessage); !ok {
		t.Fatal("unexpected type")
	} else {
		assert.Equal(t, newBlock.Header().Number(), transferMsg.Meta.BlockNumber)
		assert.Equal(t, newBlock.Header().ID(), transferMsg.Meta.BlockID)
		assert.Equal(t, newBlock.Header().Timestamp(), transferMsg.Meta.BlockTimestamp)
		assert.Equal(t, newBlock.Transactions()[0].ID(), transferMsg.Meta.TxID)
	}
}

func TestTransferReader_Read_NoNewBlocksToRead(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	newBlock := allBlocks[1]
	filter := &TransferFilter{}

	// Act
	br := newTransferReader(thorChain.Repo(), newBlock.Header().ID(), filter)
	res, ok, err := br.Read()

	// Assert
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func TestTransferReader_Read_ErrorWhenReadingBlocks(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	filter := &TransferFilter{}

	// Act
	br := newTransferReader(thorChain.Repo(), thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), filter)
	res, ok, err := br.Read()

	// Assert
	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func TestTransferReader_Read_NoTransferMatchingTheFilter(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	genesisBlk := allBlocks[0]

	nonExistingAddress := thor.MustParseAddress("0xffffffffffffffffffffffffffffffffffffffff")
	badFilter := &TransferFilter{
		TxOrigin: &nonExistingAddress,
	}

	// Act
	br := newTransferReader(thorChain.Repo(), genesisBlk.Header().ID(), badFilter)
	res, ok, err := br.Read()

	// Assert
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, res)
}
