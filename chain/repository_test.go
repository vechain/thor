// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"bytes"
	"sort"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	. "github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
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
	//		Genesis
	//		|
	//		|
	// 		b0 (+10)
	// 		|
	//		|
	//		b1 (+20)
	//		|----------------
	// 		|				|
	// 		b2 (+30) 		b5 (+30)
	// 		|----------------
	//		|				|
	//		b3 (+40)		b4 (+50)

	var (
		branches []*chain.Chain
		expected []thor.Bytes32
		actual   []thor.Bytes32
	)

	repo := newTestRepo()

	blocks := make([]*block.Block, 6)
	launchTime := repo.GenesisBlock().Header().Timestamp()
	blocks[0] = newBlock(repo.GenesisBlock(), 10+launchTime)
	blocks[1] = newBlock(blocks[0], 20+launchTime)
	blocks[2] = newBlock(blocks[1], 30+launchTime)
	blocks[3] = newBlock(blocks[2], 40+launchTime)
	blocks[4] = newBlock(blocks[2], 50+launchTime)
	blocks[5] = newBlock(blocks[1], 30+launchTime)

	for _, blk := range blocks {
		err := repo.AddBlock(blk, nil)
		assert.Nil(t, err)
	}

	branches = repo.GetBranchesByID(blocks[1].Header().ID())
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

	branches = repo.GetBranchesByTimestamp(20 + launchTime)
	assert.Equal(t, 3, len(branches))
	expected = []thor.Bytes32{blocks[5].Header().ID(), blocks[3].Header().ID(), blocks[4].Header().ID()}
	sortBytes32Array(expected)
	actual = []thor.Bytes32{branches[0].HeadID(), branches[1].HeadID(), branches[2].HeadID()}
	sortBytes32Array(actual)
	assert.Equal(t, expected, actual)

	branches = repo.GetBranchesByTimestamp(30 + launchTime)
	assert.Equal(t, 2, len(branches))
	expected = []thor.Bytes32{blocks[3].Header().ID(), blocks[4].Header().ID()}
	sortBytes32Array(expected)
	actual = []thor.Bytes32{branches[0].HeadID(), branches[1].HeadID()}
	sortBytes32Array(actual)
	assert.Equal(t, expected, actual)

	branches = repo.GetBranchesByTimestamp(40 + launchTime)
	assert.Equal(t, 1, len(branches))
	expected = []thor.Bytes32{blocks[4].Header().ID()}
	actual = []thor.Bytes32{branches[0].HeadID()}
	assert.Equal(t, expected, actual)

	branches = repo.GetBranchesByTimestamp(50 + launchTime)
	assert.Nil(t, branches)
}

func sortBytes32Array(a []thor.Bytes32) {
	sort.Slice(a, func(i, j int) bool {
		return bytes.Compare(a[i][:], a[j][:]) < 0
	})
}
