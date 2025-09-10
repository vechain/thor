// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func M(args ...any) []any {
	return args
}

func newTestRepo() (*muxdb.MuxDB, *Repository) {
	db := muxdb.NewMem()
	b0 := new(block.Builder).
		ParentID(thor.Bytes32{0xff, 0xff, 0xff, 0xff}).
		Build()

	repo, err := NewRepository(db, b0)
	if err != nil {
		panic(err)
	}
	return db, repo
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

func TestRepositoryFunc(t *testing.T) {
	db, repo1 := newTestRepo()
	b0 := repo1.GenesisBlock()

	repo1, err := NewRepository(db, b0)
	if err != nil {
		panic(err)
	}
	b0summary, _ := repo1.GetBlockSummary(b0.Header().ID())
	assert.Equal(t, b0summary, repo1.BestBlockSummary())
	assert.Equal(t, repo1.GenesisBlock().Header().ID()[31], repo1.ChainTag())

	tx1 := new(tx.Builder).Build()
	receipt1 := &tx.Receipt{}

	b1 := newBlock(repo1.GenesisBlock(), 10, tx1)
	assert.Nil(t, repo1.AddBlock(b1, tx.Receipts{receipt1}, 0, false))
	// best block not set, so still 0
	assert.Equal(t, uint32(0), repo1.BestBlockSummary().Header.Number())

	assert.Nil(t, repo1.AddBlock(b1, tx.Receipts{receipt1}, 0, true))
	assert.Equal(t, uint32(1), repo1.BestBlockSummary().Header.Number())

	repo2, _ := NewRepository(db, b0)
	for _, repo := range []*Repository{repo1, repo2} {
		assert.Equal(t, b1.Header().ID(), repo.BestBlockSummary().Header.ID())
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

func TestAddBlock(t *testing.T) {
	_, repo := newTestRepo()

	err := repo.AddBlock(new(block.Builder).Build(), nil, 0, false)
	assert.Error(t, err, "parent missing")

	b1 := newBlock(repo.GenesisBlock(), 10)
	assert.Nil(t, repo.AddBlock(b1, nil, 0, false))
}

func TestConflicts(t *testing.T) {
	_, repo := newTestRepo()
	b0 := repo.GenesisBlock()

	b1 := newBlock(b0, 10)
	repo.AddBlock(b1, nil, 0, false)

	assert.Equal(t, []any{uint32(1), nil}, M(repo.GetMaxBlockNum()))
	assert.Equal(t, []any{uint32(1), nil}, M(repo.ScanConflicts(1)))
	assert.Equal(t, []any{[]thor.Bytes32{b1.Header().ID()}, nil}, M(repo.GetConflicts(1)))

	b1x := newBlock(b0, 20)
	repo.AddBlock(b1x, nil, 1, false)
	assert.Equal(t, []any{uint32(1), nil}, M(repo.GetMaxBlockNum()))
	assert.Equal(t, []any{uint32(2), nil}, M(repo.ScanConflicts(1)))
	switch bytes.Compare(b1.Header().ID().Bytes(), b1x.Header().ID().Bytes()) {
	case -1:
		assert.Equal(t, []any{[]thor.Bytes32{b1.Header().ID(), b1x.Header().ID()}, nil}, M(repo.GetConflicts(1)))
	case 1:
		assert.Equal(t, []any{[]thor.Bytes32{b1x.Header().ID(), b1.Header().ID()}, nil}, M(repo.GetConflicts(1)))
	}
}

func TestScanHeads(t *testing.T) {
	_, repo := newTestRepo()

	heads, err := repo.ScanHeads(0)
	assert.Nil(t, err)

	assert.Equal(t, []thor.Bytes32{repo.GenesisBlock().Header().ID()}, heads)

	b1 := newBlock(repo.GenesisBlock(), 10)
	err = repo.AddBlock(b1, nil, 0, false)
	assert.Nil(t, err)
	heads, err = repo.ScanHeads(0)
	assert.Nil(t, err)
	assert.Equal(t, []thor.Bytes32{b1.Header().ID()}, heads)

	b2 := newBlock(b1, 20)
	err = repo.AddBlock(b2, nil, 0, false)
	assert.Nil(t, err)
	heads, err = repo.ScanHeads(0)
	assert.Nil(t, err)
	assert.Equal(t, []thor.Bytes32{b2.Header().ID()}, heads)

	heads, err = repo.ScanHeads(10)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(heads))

	b2x := newBlock(b1, 20)
	err = repo.AddBlock(b2x, nil, 0, false)
	assert.Nil(t, err)
	heads, err = repo.ScanHeads(0)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(heads))
	if heads[0] == b2.Header().ID() {
		assert.Equal(t, []thor.Bytes32{b2.Header().ID(), b2x.Header().ID()}, heads)
	} else {
		assert.Equal(t, []thor.Bytes32{b2x.Header().ID(), b2.Header().ID()}, heads)
	}

	b3 := newBlock(b2, 30)
	err = repo.AddBlock(b3, nil, 0, false)
	assert.Nil(t, err)
	heads, err = repo.ScanHeads(0)
	assert.Nil(t, err)
	assert.Equal(t, []thor.Bytes32{b3.Header().ID(), b2x.Header().ID()}, heads)

	heads, err = repo.ScanHeads(2)
	assert.Nil(t, err)
	assert.Equal(t, []thor.Bytes32{b3.Header().ID(), b2x.Header().ID()}, heads)

	heads, err = repo.ScanHeads(3)
	assert.Nil(t, err)
	assert.Equal(t, []thor.Bytes32{b3.Header().ID()}, heads)

	b3x := newBlock(b2, 30)
	err = repo.AddBlock(b3x, nil, 0, false)
	assert.Nil(t, err)
	heads, err = repo.ScanHeads(0)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(heads))
	if heads[0] == b3.Header().ID() {
		assert.Equal(t, []thor.Bytes32{b3.Header().ID(), b3x.Header().ID(), b2x.Header().ID()}, heads)
	} else {
		assert.Equal(t, []thor.Bytes32{b3x.Header().ID(), b3.Header().ID(), b2x.Header().ID()}, heads)
	}
}

type errorStore struct {
	putErr    error
	getErr    error
	deleteErr error
	data      map[string][]byte
}

func (e *errorStore) Put(key, value []byte) error {
	if e.putErr != nil {
		return e.putErr
	}
	if e.data == nil {
		e.data = make(map[string][]byte)
	}
	e.data[string(key)] = value
	return nil
}

func (e *errorStore) Get(key []byte) ([]byte, error) {
	if e.getErr != nil {
		return nil, e.getErr
	}
	if e.data == nil {
		return nil, nil
	}
	return e.data[string(key)], nil
}

func (e *errorStore) Delete(key []byte) error {
	if e.deleteErr != nil {
		return e.deleteErr
	}
	if e.data != nil {
		delete(e.data, string(key))
	}
	return nil
}

func (e *errorStore) DeleteRange(ctx context.Context, r kv.Range) error { return nil }
func (e *errorStore) Iterate(_ kv.Range) kv.Iterator                    { return nil }
func (e *errorStore) IsNotFound(err error) bool                         { return err == assert.AnError }
func (e *errorStore) Bulk() kv.Bulk                                     { return nil }
func (e *errorStore) Has(key []byte) (bool, error)                      { return false, e.getErr }
func (e *errorStore) Snapshot() kv.Snapshot                             { return nil }

func TestRepository_ErrorBranches(t *testing.T) {
	_, repo := newTestRepo()
	repo.bodyStore = &errorStore{getErr: assert.AnError}
	repo.hdrStore = &errorStore{getErr: assert.AnError}
	repo.txIndexer = &errorStore{getErr: assert.AnError}

	t.Run("getTransaction error", func(t *testing.T) {
		_, err := repo.getTransaction([]byte("key"))
		assert.Error(t, err)
	})
	t.Run("getReceipt error", func(t *testing.T) {
		_, err := repo.getReceipt([]byte("key"))
		assert.Error(t, err)
	})
	t.Run("loadTransaction decode error", func(t *testing.T) {
		es := &errorStore{data: map[string][]byte{"key": {0xff, 0xff}}}
		_, err := loadTransaction(es, []byte("key"))
		assert.Error(t, err)
	})
	t.Run("loadReceipt decode error", func(t *testing.T) {
		es := &errorStore{data: map[string][]byte{"key": {0xff, 0xff}}}
		_, err := loadReceipt(es, []byte("key"))
		assert.Error(t, err)
	})

	t.Run("AddBlock parent error", func(t *testing.T) {
		repo2 := repo
		repo2.hdrStore = &errorStore{getErr: assert.AnError}
		b := new(block.Builder).Build()
		err := repo2.AddBlock(b, nil, 0, false)
		assert.Error(t, err)
	})
}
