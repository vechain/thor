// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func newTx() *tx.Transaction {
	tx := new(tx.Builder).Build()
	pk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), pk)
	return tx.WithSignature(sig)
}

func TestChain(t *testing.T) {
	tx1 := newTx()

	_, repo := newTestRepo()

	b1 := newBlock(repo.GenesisBlock(), 10, tx1)
	tx1Meta := &chain.TxMeta{BlockID: b1.Header().ID(), Index: 0, Reverted: false}
	tx1Receipt := &tx.Receipt{}
	repo.AddBlock(b1, tx.Receipts{tx1Receipt}, 0)

	b2 := newBlock(b1, 20)
	repo.AddBlock(b2, nil, 0)

	b3 := newBlock(b2, 30)
	repo.AddBlock(b3, nil, 0)

	b3x := newBlock(b2, 30)
	repo.AddBlock(b3x, nil, 1)

	c := repo.NewChain(b3.Header().ID())

	assert.Equal(t, b3.Header().ID(), c.HeadID())
	assert.Equal(t, M(b3.Header().ID(), nil), M(c.GetBlockID(3)))
	assert.Equal(t, M(b3.Header(), nil), M(c.GetBlockHeader(3)))
	assert.Equal(t, M(block.Compose(b3.Header(), b3.Transactions()), nil), M(c.GetBlock(3)))

	_, err := c.GetBlockID(4)
	assert.True(t, c.IsNotFound(err))

	assert.Equal(t, M(tx1Meta, nil), M(c.GetTransactionMeta(tx1.ID())))
	assert.Equal(t, M(tx1, tx1Meta, nil), M(c.GetTransaction(tx1.ID())))
	assert.Equal(t, M(tx1Receipt, nil), M(c.GetTransactionReceipt(tx1.ID())))
	_, err = c.GetTransactionMeta(thor.Bytes32{})
	assert.True(t, c.IsNotFound(err))

	assert.Equal(t, M(true, nil), M(c.HasBlock(b1.Header().ID())))
	assert.Equal(t, M(false, nil), M(c.HasBlock(b3x.Header().ID())))

	assert.Equal(t, M(b3.Header(), nil), M(c.FindBlockHeaderByTimestamp(25, 1)))
	assert.Equal(t, M(b2.Header(), nil), M(c.FindBlockHeaderByTimestamp(25, -1)))
	_, err = c.FindBlockHeaderByTimestamp(25, 0)
	assert.True(t, c.IsNotFound(err))

	c1, c2 := repo.NewChain(b3.Header().ID()), repo.NewChain(b3x.Header().ID())

	assert.Equal(t, M([]thor.Bytes32{b3.Header().ID()}, nil), M(c1.Exclude(c2)))
	assert.Equal(t, M([]thor.Bytes32{b3x.Header().ID()}, nil), M(c2.Exclude(c1)))

	dangleID := thor.Bytes32{0, 0, 0, 4}
	dangleChain := repo.NewChain(dangleID)

	_, err = c1.Exclude(dangleChain)
	assert.Error(t, err)

	_, err = dangleChain.Exclude(c1)
	assert.Error(t, err)
}
