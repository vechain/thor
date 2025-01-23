// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
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
	tx1Meta := &TxMeta{BlockNum: 1, Index: 0, Reverted: false}
	tx1Receipt := &tx.Receipt{}
	repo.AddBlock(b1, tx.Receipts{tx1Receipt}, 0, false)

	b2 := newBlock(b1, 20)
	repo.AddBlock(b2, nil, 0, false)

	b3 := newBlock(b2, 30)
	repo.AddBlock(b3, nil, 0, false)

	b3x := newBlock(b2, 30)
	repo.AddBlock(b3x, nil, 1, false)

	c := repo.NewChain(b3.Header().ID())

	assert.Equal(t, b3.Header().ID(), c.HeadID())
	assert.Equal(t, M(b3.Header().ID(), nil), M(c.GetBlockID(3)))
	assert.Equal(t, M(b3.Header(), nil), M(c.GetBlockHeader(3)))
	assert.Equal(t, M(block.Compose(b3.Header(), b3.Transactions()), nil), M(c.GetBlock(3)))
	assert.Equal(t, repo.NewBestChain().GenesisID(), repo.GenesisBlock().Header().ID())

	_, err := c.GetBlockID(4)
	assert.True(t, c.IsNotFound(err))

	assert.Equal(t, M(tx1Meta, nil), M(c.GetTransactionMeta(tx1.ID())))
	{
		tx, meta, err := c.GetTransaction(tx1.ID())
		assert.Nil(t, err)
		assert.Equal(t, tx1Meta, meta)
		assert.Equal(t, tx1.ID(), tx.ID())
	}
	{
		r, err := c.GetTransactionReceipt(tx1.ID())
		assert.Nil(t, err)
		got, _ := rlp.EncodeToBytes(r)
		want, _ := rlp.EncodeToBytes(tx1Receipt)
		assert.Equal(t, want, got)
	}

	_, err = c.GetTransactionMeta(thor.Bytes32{})
	assert.True(t, c.IsNotFound(err))

	assert.Equal(t, M(true, nil), M(c.HasTransaction(tx1.ID(), tx1.BlockRef().Number())))
	assert.Equal(t, M(false, nil), M(c.HasTransaction(tx1.ID(), block.Number(c.HeadID()))))
	assert.Equal(t, M(false, nil), M(c.HasTransaction(thor.Bytes32{}, 0)))

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

func TestHasTransaction(t *testing.T) {
	_, repo := newTestRepo()

	parent := repo.GenesisBlock()
	for i := 1; i <= 101; i++ {
		b := newBlock(parent, uint64(i)*10)
		asBest := i == 101
		repo.AddBlock(b, nil, 0, asBest)
		parent = b
	}

	has, err := repo.NewBestChain().HasTransaction(datagen.RandomHash(), 0)
	assert.Nil(t, err)
	assert.False(t, has)

	tx1 := newTx()
	bx := newBlock(parent, 10020, tx1)
	repo.AddBlock(bx, tx.Receipts{&tx.Receipt{}}, 0, true)

	has, err = repo.NewBestChain().HasTransaction(tx1.ID(), 0)
	assert.Nil(t, err)
	assert.True(t, has)
}
