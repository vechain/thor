// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestTransferReader_Read(t *testing.T) {
	repo, generatedBlocks := initChain(t)
	genesisBlk := generatedBlocks[0]
	newBlock := generatedBlocks[1]
	filter := &TransferFilter{}

	// Test case 1: Successful read next blocks transfer
	br := newTransferReader(repo, genesisBlk.Header().ID(), filter)
	res, ok, err := br.Read()

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

	// Test case 2: There is no new block
	br = newTransferReader(repo, newBlock.Header().ID(), filter)
	res, ok, err = br.Read()

	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)

	// Test case 3: Error when reading blocks
	br = newTransferReader(repo, thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), filter)
	res, ok, err = br.Read()

	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)

	// Test case 4: There is no transfer matching the filter
	nonExistingAddress := thor.MustParseAddress("0xffffffffffffffffffffffffffffffffffffffff")
	badFilter := &TransferFilter{
		TxOrigin: &nonExistingAddress,
	}
	br = newTransferReader(repo, genesisBlk.Header().ID(), badFilter)
	res, ok, err = br.Read()

	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, res)
}
