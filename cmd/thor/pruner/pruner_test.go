// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pruner

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vechain/thor/v2/bft"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func fastForwardTo(from uint32, to uint32, db *muxdb.MuxDB) (thor.Bytes32, error) {
	var (
		parentID thor.Bytes32
		id       thor.Bytes32
	)
	binary.BigEndian.PutUint32(parentID[:], to-1)
	binary.BigEndian.PutUint32(id[:], to)

	blk := new(block.Builder).ParentID(parentID).Build()
	summary := &chain.BlockSummary{
		Header:    blk.Header(),
		Conflicts: 0,
	}

	data, err := rlp.EncodeToBytes(summary)
	if err != nil {
		return thor.Bytes32{}, err
	}

	store := db.NewStore("chain.hdr")
	err = store.Put(id.Bytes(), data)
	if err != nil {
		return thor.Bytes32{}, err
	}

	indexTrie := db.NewTrie("i", trie.Root{
		Hash: thor.BytesToBytes32([]byte{1}),
		Ver: trie.Version{
			Major: from,
			Minor: 0,
		},
	})
	if err := indexTrie.Update(id[:4], id[:], nil); err != nil {
		return thor.Bytes32{}, err
	}

	if err := indexTrie.Commit(trie.Version{Major: to, Minor: 0}, true); err != nil {
		return thor.Bytes32{}, err
	}
	return id, nil
}

func newBlock(parentID thor.Bytes32, score uint64, stateRoot thor.Bytes32, priv *ecdsa.PrivateKey) *block.Block {
	now := uint64(time.Now().Unix())
	blk := new(block.Builder).ParentID(parentID).TotalScore(score).StateRoot(stateRoot).Timestamp(now - now%10 - 10).Build()

	if priv != nil {
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), priv)
		return blk.WithSignature(sig)
	}
	return blk
}

func TestStatus(t *testing.T) {
	store := muxdb.NewMem().NewStore("test")

	s := &status{}
	err := s.Load(store)
	assert.Nil(t, err, "load should not error")
	assert.Equal(t, uint32(0), s.Base)

	s.Base = 1

	err = s.Save(store)
	assert.Nil(t, err, "save should not error")

	s2 := &status{}
	err = s2.Load(store)
	assert.Nil(t, err, "load should not error")
	assert.Equal(t, uint32(1), s.Base)
}

func TestNewPruner(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()
	b0, _, _, _ := gene.Build(stater)
	repo, _ := chain.NewRepository(db, b0)

	bftMockedEngine := bft.NewMockedEngine(repo.GenesisBlock().Header().ID())
	pr := New(db, repo, bftMockedEngine)
	pr.Stop()
}

func newTempFileDB() (*muxdb.MuxDB, func() error, error) {
	dir := os.TempDir()

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        128,
		TrieCachedNodeTTL:          30, // 5min
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       true,
		OpenFilesCacheCapacity:     512,
		ReadCacheMB:                256, // rely on os page cache other than huge db read cache.
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    1000,
	}
	path := filepath.Join(dir, "main.db")
	db, err := muxdb.Open(path, &opts)
	if err != nil {
		return nil, nil, err
	}

	close := func() error {
		err = db.Close()
		if err != nil {
			return err
		}
		err = os.RemoveAll(path)
		if err != nil {
			return err
		}
		return nil
	}

	return db, close, nil
}

type testCommitter struct {
	repo *chain.Repository
}

func (tc *testCommitter) Finalized() thor.Bytes32 {
	best := tc.repo.BestBlockSummary()
	return best.Header.ID()
}

func (tc *testCommitter) Justified() (thor.Bytes32, error) {
	best := tc.repo.BestBlockSummary()
	return best.Header.ID(), nil
}

func (tc *testCommitter) Accepts(parentID thor.Bytes32) (bool, error) {
	// For testing, accept all blocks (they're all on the same chain)
	return true, nil
}

func (tc *testCommitter) Select(header *block.Header) (bool, error) {
	// For testing, always select the new block
	return true, nil
}

func (tc *testCommitter) CommitBlock(header *block.Header, isPacking bool) error {
	// For testing, no-op
	return nil
}

func (tc *testCommitter) ShouldVote(parentID thor.Bytes32) (bool, error) {
	// For testing, don't vote
	return false, nil
}

func TestWaitUntil(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()
	b0, _, _, _ := gene.Build(stater)
	repo, _ := chain.NewRepository(db, b0)
	devAccounts := genesis.DevAccounts()
	testCommiter := &testCommitter{repo: repo}

	ctx, cancel := context.WithCancel(context.Background())
	pruner := &Pruner{
		repo:     repo,
		db:       db,
		ctx:      ctx,
		commiter: testCommiter,
		cancel:   cancel,
	}

	parentID := b0.Header().ID()
	var parentScore uint64 = 0
	for range 6 {
		blk := newBlock(parentID, parentScore+2, b0.Header().StateRoot(), devAccounts[0].PrivateKey)
		err := repo.AddBlock(blk, tx.Receipts{}, 0, true)
		assert.Nil(t, err)

		parentID = blk.Header().ID()
		parentScore = blk.Header().TotalScore()
	}

	parentID, err := fastForwardTo(block.Number(parentID), 100000-1, db)
	assert.Nil(t, err)

	parentScore = (100000 - 1) * 2
	for range 3 {
		signer := devAccounts[0].PrivateKey
		score := parentScore + 1
		blk := newBlock(parentID, score, b0.Header().StateRoot(), signer)
		err := repo.AddBlock(blk, tx.Receipts{}, 0, true)
		assert.Nil(t, err)

		parentID = blk.Header().ID()
		parentScore = blk.Header().TotalScore()
	}

	cancel()
	// Use a target that doesn't exist yet to force waiting (where cancellation is checked)
	_, err = pruner.awaitUntilSteady(200000) // Target beyond current best
	assert.NotNil(t, err)
	assert.Equal(t, context.Canceled, err)

	for i := range 3 {
		signer := devAccounts[i%2].PrivateKey
		score := parentScore + 2
		blk := newBlock(parentID, score, b0.Header().StateRoot(), signer)

		err := repo.AddBlock(blk, tx.Receipts{}, 0, true)
		assert.Nil(t, err)
		parentID = blk.Header().ID()
		parentScore = blk.Header().TotalScore()
	}

	ctx, cancel = context.WithCancel(context.Background())
	pruner.ctx = ctx
	pruner.cancel = cancel

	chain, err := pruner.awaitUntilSteady(100000)
	assert.Nil(t, err)

	assert.True(t, block.Number(chain.HeadID()) >= 10000)
}

//func TestWaitUntil(t *testing.T) {
//	db := muxdb.NewMem()
//	stater := state.NewStater(db)
//	gene := genesis.NewDevnet()
//	b0, _, _, _ := gene.Build(stater)
//	repo, _ := chain.NewRepository(db, b0)
//	devAccounts := genesis.DevAccounts()
//
//	testCommiter := &testCommitter{repo: repo}
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	pruner := &Pruner{
//		repo:     repo,
//		db:       db,
//		ctx:      ctx,
//		commiter: testCommiter,
//		cancel:   cancel,
//	}
//
//	parentID := b0.Header().ID()
//	var parentScore uint64 = 0
//
//	// Add a reasonable number of blocks (e.g., 1000) instead of 100000
//	for i := uint32(0); i < 1000; i++ {
//		blk := newBlock(parentID, parentScore+2, b0.Header().StateRoot(), devAccounts[i%uint32(len(devAccounts))].PrivateKey)
//		err := repo.AddBlock(blk, tx.Receipts{}, 0, true)
//		assert.Nil(t, err)
//
//		parentID = blk.Header().ID()
//		parentScore = blk.Header().TotalScore()
//	}
//
//	// Verify best block is at 1000
//	best := repo.BestBlockSummary()
//	assert.Equal(t, uint32(1000), best.Header.Number())
//
//	// Verify we can actually get block 500 from the chain before testing awaitUntilSteady
//	bestChain := repo.NewBestChain()
//	block500ID, err := bestChain.GetBlockID(500)
//	assert.Nil(t, err, "Should be able to get block 500 ID - this verifies the index trie is properly set up")
//	assert.NotEqual(t, thor.Bytes32{}, block500ID, "Block 500 ID should not be zero")
//
//	// Test with target that's less than finalized - should succeed immediately
//	// Note: The awaitUntilSteady function has a logic issue where it checks if finalized
//	// is on the target chain, which will always fail when target < finalized.
//	// Since we can't change the function, we'll test with target == finalized instead
//	target := uint32(1000) // Use finalized block number instead of 500
//
//	// Use a timeout context to prevent hanging
//	ctxWithTimeout, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancelTimeout()
//	pruner.ctx = ctxWithTimeout
//
//	chain, err := pruner.awaitUntilSteady(target)
//	if !assert.Nil(t, err) {
//		t.Fatalf("awaitUntilSteady failed: %v", err)
//	}
//	if !assert.NotNil(t, chain) {
//		t.Fatal("awaitUntilSteady returned nil chain")
//	}
//	assert.True(t, block.Number(chain.HeadID()) >= target)
//}

func TestPrune(t *testing.T) {
	db, closeDB, err := newTempFileDB()
	assert.Nil(t, err)

	stater := state.NewStater(db)
	gene := genesis.NewDevnet()
	b0, _, _, _ := gene.Build(stater)
	repo, _ := chain.NewRepository(db, b0)
	devAccounts := genesis.DevAccounts()

	ctx, cancel := context.WithCancel(context.Background())
	pruner := &Pruner{
		repo:   repo,
		db:     db,
		ctx:    ctx,
		cancel: cancel,
	}

	acc1 := thor.BytesToAddress([]byte("account1"))
	acc2 := thor.BytesToAddress([]byte("account2"))
	key := thor.BytesToBytes32([]byte("key"))
	value := thor.BytesToBytes32([]byte("value"))
	code := []byte("code")

	parentID := b0.Header().ID()
	for range 9 {
		blk := newBlock(parentID, 10, b0.Header().StateRoot(), nil)

		err := repo.AddBlock(blk, tx.Receipts{}, 0, false)
		assert.Nil(t, err)
		parentID = blk.Header().ID()
	}

	st := stater.NewState(trie.Root{Hash: b0.Header().StateRoot(), Ver: trie.Version{Major: 0, Minor: 0}})
	st.SetBalance(acc1, big.NewInt(1e18))
	st.SetCode(acc2, code)
	st.SetStorage(acc2, key, value)
	stage, err := st.Stage(trie.Version{Major: 10, Minor: 0})
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	blk := newBlock(parentID, 10, root, devAccounts[0].PrivateKey)
	err = repo.AddBlock(blk, tx.Receipts{}, 0, true)
	assert.Nil(t, err)
	parentID = blk.Header().ID()

	err = pruner.pruneTries(repo.NewBestChain(), 0, block.Number(parentID)+1)
	assert.Nil(t, err)

	closeDB()
}
