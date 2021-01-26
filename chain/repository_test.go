// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
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
	assert.Nil(t, repo1.AddBlock(b1, tx.Receipts{receipt1}, nil))

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

func TestBlockSummary(t *testing.T) {
	type bSummary struct {
		Header    *block.Header
		IndexRoot thor.Bytes32
		Txs       []thor.Bytes32
		Size      uint64
	}

	b := new(block.Builder).Build()
	root := b.Transactions().RootHash()
	txs := []thor.Bytes32{b.Header().ID()}
	size := uint64(b.Size().Int64())

	bs1 := bSummary{b.Header(), root, txs, size}

	bytes1, err := rlp.EncodeToBytes(bs1)
	if err != nil {
		t.Fatal(err)
	}

	var bs2 BlockSummary
	err = rlp.DecodeBytes(bytes1, &bs2)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 0, len(bs2.Beta))
	assert.Equal(t, bs1.Header.ID(), bs2.Header.ID())
	assert.Equal(t, bs1.IndexRoot, bs2.IndexRoot)
	assert.Equal(t, bs1.Size, bs2.Size)
	assert.Equal(t, bs1.Txs, bs2.Txs)

	bytes2, err := rlp.EncodeToBytes(&bs2)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, bytes1, bytes2)

	beta := []byte{0xff}
	bs3 := BlockSummary{
		bs1.Header,
		bs1.IndexRoot,
		bs1.Txs,
		bs1.Size,
		beta,
	}

	bytes3, err := rlp.EncodeToBytes(&bs3)
	if err != nil {
		t.Fatal(err)
	}

	var bs4 BlockSummary
	err = rlp.DecodeBytes(bytes3, &bs4)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, beta, []byte(bs4.Beta))
}
