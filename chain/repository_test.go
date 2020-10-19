// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
)

func M(args ...interface{}) []interface{} {
	return args
}

func newTestRepo() *Repository {
	db := muxdb.NewMem()
	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, err := NewRepository(db, b0)
	if err != nil {
		panic(err)
	}
	return repo
}

func newBlock(parent *block.Block, ts uint64, txs ...*tx.Transaction) *block.Block {
	builder := new(block.Builder).
		ParentID(parent.Header().ID()).
		Timestamp(ts)

	for _, tx := range txs {
		builder.Transaction(tx)
	}
	b := builder.Build()

	pk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), pk)
	return b.WithSignature(sig)
}

func TestRepository(t *testing.T) {
	db := muxdb.NewMem()
	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo1, err := NewRepository(db, b0)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, repo1.GenesisBlock(), repo1.BestBlock())
	assert.Equal(t, repo1.GenesisBlock().Header().ID()[31], repo1.ChainTag())

	tx1 := new(tx.Builder).Build()
	receipt1 := &tx.Receipt{}

	b1 := newBlock(repo1.GenesisBlock(), 10, tx1)
	assert.Nil(t, repo1.AddBlock(b1, tx.Receipts{receipt1}))

	// best block not set, so still 0
	assert.Equal(t, uint32(0), repo1.BestBlock().Header().Number())

	repo1.SetBestBlockID(b1.Header().ID())
	repo2, _ := NewRepository(db, b0)
	for _, repo := range []*Repository{repo1, repo2} {

		assert.Equal(t, b1.Header().ID(), repo.BestBlock().Header().ID())
		s, err := repo.GetBlockSummary(b1.Header().ID())
		assert.Nil(t, err)
		assert.Equal(t, b1.Header().ID(), s.Header.ID())
		assert.Equal(t, 1, len(s.Txs))
		assert.Equal(t, tx1.ID(), s.Txs[0])

		gotb, _ := repo.GetBlock(b1.Header().ID())
		assert.Equal(t, b1.Transactions().RootHash(), gotb.Transactions().RootHash())

		gotReceipts, _ := repo.GetBlockReceipts(b1.Header().ID())

		assert.Equal(t, tx.Receipts{receipt1}.RootHash(), gotReceipts.RootHash())
	}
}

func TestBranchOps(t *testing.T) {
	// 		b0
	// 		|
	//		|
	//		b1
	//		|--------
	// 		|		|
	// 		b2 		b5
	// 		|--------
	//		|		|
	//		b3		b4

	repo := newTestRepo()

	blocks := make([]*block.Block, 6)
	blocks[0] = newBlock(repo.GenesisBlock(), 10)
	blocks[1] = newBlock(blocks[0], 20)
	blocks[2] = newBlock(blocks[1], 30)
	blocks[3] = newBlock(blocks[2], 40)
	blocks[4] = newBlock(blocks[2], 40)
	blocks[5] = newBlock(blocks[1], 30)

	for _, blk := range blocks {
		err := repo.AddBlock(blk, nil)
		assert.Nil(t, err)
	}

	branches := repo.GetBranches(blocks[1].Header().ID())

	assert.Equal(t, len(branches), 3)
	assert.Equal(t, branches[0].HeadID(), blocks[5].Header().ID())

	var b0, b1 *block.Block
	if bytes.Compare(blocks[3].Header().ID().Bytes(), blocks[4].Header().ID().Bytes()) < 0 {
		b0 = blocks[3]
		b1 = blocks[4]
	} else {
		b0 = blocks[4]
		b1 = blocks[3]
	}
	assert.Equal(t, branches[1].HeadID(), b0.Header().ID())
	assert.Equal(t, branches[2].HeadID(), b1.Header().ID())
}
