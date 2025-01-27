// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/eventcontract"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestBlockReader_Read(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	genesisBlk := allBlocks[0]
	newBlock := allBlocks[1]

	// Test case 1: Successful read next blocks
	br := newBlockReader(thorChain.Repo(), genesisBlk.Header().ID())
	res, ok, err := br.Read()

	assert.NoError(t, err)
	assert.True(t, ok)
	if resBlock, ok := res[0].(*BlockMessage); !ok {
		t.Fatal("unexpected type")
	} else {
		assert.Equal(t, newBlock.Header().Number(), resBlock.Number)
		assert.Equal(t, newBlock.Header().ParentID(), resBlock.ParentID)
	}

	// Test case 2: There is no new block
	br = newBlockReader(thorChain.Repo(), newBlock.Header().ID())
	res, ok, err = br.Read()

	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)

	// Test case 3: Error when reading blocks
	br = newBlockReader(thorChain.Repo(), thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
	res, ok, err = br.Read()

	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func initChain(t *testing.T) *testchain.Chain {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tr := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)

	txDeploy := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(100).
		Gas(1_000_000).
		Nonce(3).
		Clause(tx.NewClause(nil).WithData(common.Hex2Bytes(eventcontract.HexBytecode))).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	sigTxDeploy, err := crypto.Sign(txDeploy.SigningHash().Bytes(), genesis.DevAccounts()[1].PrivateKey)
	require.NoError(t, err)
	txDeploy = txDeploy.WithSignature(sigTxDeploy)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], tr, txDeploy))

	return thorChain
}
