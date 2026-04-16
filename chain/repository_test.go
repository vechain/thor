// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// ethChainID is an arbitrary Ethereum replay-protection chain ID used in repository tests.
// TODO: revisit once chain ID is derivable from network config rather than hard-coded.
const ethChainID = uint64(1337)

// ethRepoTestKey is a deterministic secp256k1 private key for signing Ethereum txs in
// repository tests.  Using a fixed key makes ethTxHash values stable across test runs.
var ethRepoTestKey, _ = crypto.HexToECDSA("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

// TestRepository_EthReceiptColdRead verifies that Ethereum-typed receipts (TypeEthLegacy
// and TypeEthTyped1559) survive a full storage round-trip when the in-memory cache is
// cold — the scenario that occurs on every node restart.
//
// Setup: repo1 adds a block containing one EthLegacy tx and one EthTyped1559 tx, each
// with a matching receipt.  repo2 is opened from the same underlying database with an
// empty cache, simulating the node restarting.  All receipt reads on repo2 must go
// through loadRLP → DecodeRLP — the path that was broken before the fix.
func TestRepository_EthReceiptColdRead(t *testing.T) {
	db, repo1 := newTestRepo()
	b0 := repo1.GenesisBlock()

	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")

	// Build a real EthLegacy tx so its ID (= ethTxHash) is indexed in the tx store.
	ethLegacyTx, err := tx.NewEthBuilder(tx.TypeEthLegacy).
		ChainID(ethChainID). // TODO: derive from network config
		Nonce(1).
		GasPrice(big.NewInt(20e9)).
		GasLimit(21000).
		To(&to).
		Value(big.NewInt(0)).
		Build(ethRepoTestKey)
	require.NoError(t, err)

	// Build a real EthTyped1559 tx.
	eth1559Tx, err := tx.NewEthBuilder(tx.TypeEthTyped1559).
		ChainID(ethChainID). // TODO: derive from network config
		Nonce(2).
		MaxPriorityFeePerGas(big.NewInt(1e9)).
		MaxFeePerGas(big.NewInt(10e9)).
		GasLimit(21000).
		To(&to).
		Value(big.NewInt(0)).
		Build(ethRepoTestKey)
	require.NoError(t, err)

	b1 := newBlock(b0, 10, ethLegacyTx, eth1559Tx)

	// Craft receipts that positionally match the transactions.
	legacyReceipt := &tx.Receipt{
		Type:     tx.TypeEthLegacy,
		GasUsed:  21000,
		GasPayer: thor.Address{},
		Paid:     big.NewInt(420e9),
		Reward:   big.NewInt(0),
		Reverted: false,
		Outputs:  []*tx.Output{},
	}
	receipt1559 := &tx.Receipt{
		Type:     tx.TypeEthTyped1559,
		GasUsed:  21000,
		GasPayer: thor.Address{},
		Paid:     big.NewInt(210e9),
		Reward:   big.NewInt(21e9),
		Reverted: false,
		Outputs:  []*tx.Output{},
	}
	receipts := tx.Receipts{legacyReceipt, receipt1559}

	require.NoError(t, repo1.AddBlock(b1, receipts, 0, true))

	// Open a fresh repository from the same database — this simulates a node restart.
	// repo2's in-memory receipt cache is empty; every read goes through the storage
	// decode path (loadRLP → DecodeRLP), which was broken for Ethereum types before fix.
	repo2, err := NewRepository(db, b0)
	require.NoError(t, err)

	// --- GetBlockReceipts: reads all receipts for the block by position ---
	gotReceipts, err := repo2.GetBlockReceipts(b1.Header().ID())
	require.NoError(t, err)
	require.Len(t, gotReceipts, 2)

	assert.Equal(t, tx.TypeEthLegacy, gotReceipts[0].Type)
	assert.Equal(t, uint64(21000), gotReceipts[0].GasUsed)
	assert.Equal(t, big.NewInt(420e9), gotReceipts[0].Paid)

	assert.Equal(t, tx.TypeEthTyped1559, gotReceipts[1].Type)
	assert.Equal(t, uint64(21000), gotReceipts[1].GasUsed)
	assert.Equal(t, big.NewInt(21e9), gotReceipts[1].Reward)

	// --- GetTransactionReceipt: resolves receipt via tx ID (= ethTxHash) ---
	// This exercises the chain-trie path: ethTxHash → tx position → receipt key → decode.
	chain2 := repo2.NewBestChain()

	gotLegacyReceipt, err := chain2.GetTransactionReceipt(ethLegacyTx.ID())
	require.NoError(t, err)
	assert.Equal(t, tx.TypeEthLegacy, gotLegacyReceipt.Type)
	assert.Equal(t, big.NewInt(420e9), gotLegacyReceipt.Paid)

	got1559Receipt, err := chain2.GetTransactionReceipt(eth1559Tx.ID())
	require.NoError(t, err)
	assert.Equal(t, tx.TypeEthTyped1559, got1559Receipt.Type)
	assert.Equal(t, big.NewInt(21e9), got1559Receipt.Reward)
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
