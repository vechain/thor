// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/node"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestBlockReader_Read(t *testing.T) {
	// Arrange
	thorNode := initChain(t)
	allBlocks, err := thorNode.GetAllBlocks()
	require.NoError(t, err)
	genesisBlk := allBlocks[0]
	newBlock := allBlocks[1]

	// Test case 1: Successful read next blocks
	br := newBlockReader(thorNode.Chain().Repo(), genesisBlk.Header().ID())
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
	br = newBlockReader(thorNode.Chain().Repo(), newBlock.Header().ID())
	res, ok, err = br.Read()

	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)

	// Test case 3: Error when reading blocks
	br = newBlockReader(thorNode.Chain().Repo(), thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
	res, ok, err = br.Read()

	assert.Error(t, err)
	assert.False(t, ok)
	assert.Empty(t, res)
}

func initChain(t *testing.T) *node.Node {
	thorChain, err := node.NewIntegrationTestChain()
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

	sig, err := crypto.Sign(tr.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tr = tr.WithSignature(sig)

	require.NoError(t, thorChain.MintTransactionsWithReceiptFunc(
		genesis.DevAccounts()[0],
		&node.TxAndRcpt{Transaction: tr, ReceiptFunc: insertMockOutputEvent}),
	)

	thorNode, err := new(node.Builder).
		WithChain(thorChain).
		WithAPIs().
		Build()
	require.NoError(t, err)
	return thorNode
}

// This is a helper function to forcly insert an event into the output receipts
func insertMockOutputEvent(receipts tx.Receipts) {
	oldReceipt := receipts[0]
	events := make(tx.Events, 0)
	events = append(events, &tx.Event{
		Address: thor.BytesToAddress([]byte("to")),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte("topic"))},
		Data:    []byte("data"),
	})
	outputs := &tx.Output{
		Transfers: oldReceipt.Outputs[0].Transfers,
		Events:    events,
	}
	receipts[0] = &tx.Receipt{
		Reverted: oldReceipt.Reverted,
		GasUsed:  oldReceipt.GasUsed,
		Outputs:  []*tx.Output{outputs},
		GasPayer: oldReceipt.GasPayer,
		Paid:     oldReceipt.Paid,
		Reward:   oldReceipt.Reward,
	}
}
