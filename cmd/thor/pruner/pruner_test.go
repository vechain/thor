// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pruner

// TODO: add test back
// import (
// 	"context"
// 	"crypto/ecdsa"
// 	"encoding/binary"
// 	"math"
// 	"math/big"
// 	"os"
// 	"path/filepath"
// 	"testing"

// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/ethereum/go-ethereum/rlp"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/vechain/thor/v2/block"
// 	"github.com/vechain/thor/v2/chain"
// 	"github.com/vechain/thor/v2/genesis"
// 	"github.com/vechain/thor/v2/muxdb"
// 	"github.com/vechain/thor/v2/state"
// 	"github.com/vechain/thor/v2/thor"
// 	"github.com/vechain/thor/v2/trie"
// 	"github.com/vechain/thor/v2/tx"
// )

// func fastForwardTo(from uint32, to uint32, db *muxdb.MuxDB, steadyID thor.Bytes32) (thor.Bytes32, error) {
// 	id := thor.Bytes32{}
// 	binary.BigEndian.PutUint32(id[:], to)

// 	var summary = &chain.BlockSummary{
// 		Header:    &block.Header{},
// 		Conflicts: 0,
// 		SteadyNum: block.Number(steadyID),
// 	}

// 	data, err := rlp.EncodeToBytes(summary)
// 	if err != nil {
// 		return thor.Bytes32{}, err
// 	}

// 	store := db.NewStore("chain.data")
// 	err = store.Put(id.Bytes(), data)
// 	if err != nil {
// 		return thor.Bytes32{}, err
// 	}

// 	trie := db.NewNonCryptoTrie("i", trie.NonCryptoNodeHash, from, 0)
// 	if err := trie.Update(id[:4], id[:], nil); err != nil {
// 		return thor.Bytes32{}, err
// 	}

// 	if steadyID == (thor.Bytes32{}) {
// 		if err := trie.Update(steadyID[:4], steadyID[:], nil); err != nil {
// 			return thor.Bytes32{}, err
// 		}
// 	}

// 	_, commit := trie.Stage(to, 0)
// 	err = commit()
// 	if err != nil {
// 		return thor.Bytes32{}, err
// 	}
// 	return id, nil
// }

// func newBlock(parentID thor.Bytes32, score uint64, stateRoot thor.Bytes32, priv *ecdsa.PrivateKey) *block.Block {
// 	blk := new(block.Builder).ParentID(parentID).TotalScore(score).StateRoot(stateRoot).Build()

// 	if priv != nil {
// 		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), priv)
// 		return blk.WithSignature(sig)
// 	}
// 	return blk
// }

// func TestStatus(t *testing.T) {
// 	db := muxdb.NewMem()

// 	store := db.NewStore("test")

// 	s := &status{}
// 	err := s.Load(store)
// 	assert.Nil(t, err, "load should not error")
// 	assert.Equal(t, uint32(0), s.Base)
// 	assert.Equal(t, uint32(0), s.PruneBase)

// 	s.Base = 1
// 	s.PruneBase = 2

// 	err = s.Save(store)
// 	assert.Nil(t, err, "save should not error")

// 	s2 := &status{}
// 	err = s2.Load(store)
// 	assert.Nil(t, err, "load should not error")
// 	assert.Equal(t, uint32(1), s.Base)
// 	assert.Equal(t, uint32(2), s.PruneBase)
// }

// func TestNewOptimizer(t *testing.T) {
// 	db := muxdb.NewMem()
// 	stater := state.NewStater(db)
// 	gene := genesis.NewDevnet()
// 	b0, _, _, _ := gene.Build(stater)
// 	repo, _ := chain.NewRepository(db, b0)

// 	op := New(db, repo, false)
// 	op.Stop()
// }

// func newTempFileDB() (*muxdb.MuxDB, func() error, error) {
// 	dir := os.TempDir()

// 	opts := muxdb.Options{
// 		TrieNodeCacheSizeMB:        128,
// 		TrieRootCacheCapacity:      256,
// 		TrieCachedNodeTTL:          30, // 5min
// 		TrieLeafBankSlotCapacity:   256,
// 		TrieDedupedPartitionFactor: math.MaxUint32,
// 		TrieWillCleanHistory:       true,
// 		OpenFilesCacheCapacity:     512,
// 		ReadCacheMB:                256, // rely on os page cache other than huge db read cache.
// 		WriteBufferMB:              128,
// 		TrieHistPartitionFactor:    1000,
// 	}
// 	path := filepath.Join(dir, "main.db")
// 	db, err := muxdb.Open(path, &opts)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	close := func() error {
// 		err = db.Close()
// 		if err != nil {
// 			return err
// 		}
// 		err = os.RemoveAll(path)
// 		if err != nil {
// 			return err
// 		}
// 		return nil
// 	}

// 	return db, close, nil
// }

// func TestProcessDump(t *testing.T) {
// 	db, closeDB, err := newTempFileDB()
// 	assert.Nil(t, err)
// 	stater := state.NewStater(db)
// 	gene := genesis.NewDevnet()
// 	b0, _, _, _ := gene.Build(stater)
// 	repo, _ := chain.NewRepository(db, b0)

// 	devAccounts := genesis.DevAccounts()

// 	// fast forward to 1999
// 	parentID, err := fastForwardTo(0, 1999, db, repo.SteadyBlockID())
// 	assert.Nil(t, err)

// 	var parentScore uint64 = 1999 * 2
// 	// add new blocks with signature
// 	for i := 0; i < 3; i++ {
// 		blk := newBlock(parentID, parentScore+2, b0.Header().StateRoot(), devAccounts[i%2].PrivateKey)
// 		err := repo.AddBlock(blk, tx.Receipts{}, 0)
// 		assert.Nil(t, err)

// 		parentID = blk.Header().ID()
// 		parentScore = blk.Header().TotalScore()
// 	}

// 	repo.SetBestBlockID(parentID)

// 	op := New(db, repo, false)
// 	op.Stop()

// 	var s status
// 	assert.Nil(t, s.Load(op.db.NewStore(propsStoreName)))
// 	assert.Equal(t, uint32(2000), s.Base)

// 	// fast forward to 3999
// 	parentID, err = fastForwardTo(block.Number(parentID), 3999, db, repo.SteadyBlockID())
// 	assert.Nil(t, err)

// 	// add new blocks with signature
// 	for i := 0; i < 3; i++ {
// 		blk := newBlock(parentID, parentScore+2, b0.Header().StateRoot(), devAccounts[i%2].PrivateKey)
// 		err := repo.AddBlock(blk, tx.Receipts{}, 0)
// 		assert.Nil(t, err)

// 		parentID = blk.Header().ID()
// 		parentScore = blk.Header().TotalScore()
// 	}
// 	repo.SetBestBlockID(parentID)

// 	op = New(db, repo, true)
// 	op.Stop()

// 	assert.Nil(t, s.Load(op.db.NewStore(propsStoreName)))
// 	assert.Equal(t, uint32(4000), s.Base)

// 	closeDB()
// }

// func TestWaitUntil(t *testing.T) {
// 	db := muxdb.NewMem()
// 	stater := state.NewStater(db)
// 	gene := genesis.NewDevnet()
// 	b0, _, _, _ := gene.Build(stater)
// 	repo, _ := chain.NewRepository(db, b0)
// 	devAccounts := genesis.DevAccounts()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	op := &Optimizer{
// 		repo:   repo,
// 		db:     db,
// 		ctx:    ctx,
// 		cancel: cancel,
// 	}

// 	parentID := b0.Header().ID()
// 	var parentScore uint64 = 0
// 	for i := 0; i < 6; i++ {
// 		blk := newBlock(parentID, parentScore+2, b0.Header().StateRoot(), devAccounts[0].PrivateKey)
// 		err := repo.AddBlock(blk, tx.Receipts{}, 0)
// 		assert.Nil(t, err)

// 		parentID = blk.Header().ID()
// 		parentScore = blk.Header().TotalScore()
// 	}
// 	repo.SetBestBlockID(parentID)

// 	parentID, err := fastForwardTo(block.Number(parentID), 100000-1, db, repo.SteadyBlockID())
// 	assert.Nil(t, err)

// 	parentScore = (100000 - 1) * 2
// 	for i := 0; i < 3; i++ {
// 		signer := devAccounts[0].PrivateKey
// 		score := parentScore + 1
// 		blk := newBlock(parentID, score, b0.Header().StateRoot(), signer)
// 		err := repo.AddBlock(blk, tx.Receipts{}, 0)
// 		assert.Nil(t, err)

// 		parentID = blk.Header().ID()
// 		parentScore = blk.Header().TotalScore()
// 	}
// 	repo.SetBestBlockID(parentID)

// 	go func() {
// 		cancel()
// 	}()

// 	// not enough signer, will wait for 1 sec
// 	// backoff will increase for more waiting
// 	// cancel here and restart a new test case
// 	_, err = op.awaitUntilSteady(100000)
// 	assert.NotNil(t, err)

// 	for i := 0; i < 3; i++ {
// 		signer := devAccounts[i%2].PrivateKey
// 		score := parentScore + 2
// 		blk := newBlock(parentID, score, b0.Header().StateRoot(), signer)

// 		err := repo.AddBlock(blk, tx.Receipts{}, 0)
// 		assert.Nil(t, err)
// 		parentID = blk.Header().ID()
// 		parentScore = blk.Header().TotalScore()
// 	}
// 	repo.SetBestBlockID(parentID)

// 	ctx, cancel = context.WithCancel(context.Background())
// 	op.ctx = ctx
// 	op.cancel = cancel

// 	chain, err := op.awaitUntilSteady(100000)
// 	assert.Nil(t, err)

// 	assert.True(t, block.Number(chain.HeadID()) >= 10000)
// }

// func TestDumpAndPrune(t *testing.T) {
// 	db, closeDB, err := newTempFileDB()
// 	assert.Nil(t, err)

// 	stater := state.NewStater(db)
// 	gene := genesis.NewDevnet()
// 	b0, _, _, _ := gene.Build(stater)
// 	repo, _ := chain.NewRepository(db, b0)
// 	devAccounts := genesis.DevAccounts()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	op := &Optimizer{
// 		repo:   repo,
// 		db:     db,
// 		ctx:    ctx,
// 		cancel: cancel,
// 	}

// 	acc1 := thor.BytesToAddress([]byte("account1"))
// 	acc2 := thor.BytesToAddress([]byte("account2"))
// 	key := thor.BytesToBytes32([]byte("key"))
// 	value := thor.BytesToBytes32([]byte("value"))
// 	code := []byte("code")

// 	parentID := b0.Header().ID()
// 	for i := 0; i < 9; i++ {
// 		blk := newBlock(parentID, 10, b0.Header().StateRoot(), nil)

// 		err := repo.AddBlock(blk, tx.Receipts{}, 0)
// 		assert.Nil(t, err)
// 		parentID = blk.Header().ID()
// 	}

// 	st := stater.NewState(b0.Header().StateRoot(), b0.Header().Number(), 0, 0)
// 	st.SetBalance(acc1, big.NewInt(1e18))
// 	st.SetCode(acc2, code)
// 	st.SetStorage(acc2, key, value)
// 	stage, err := st.Stage(10, 0)
// 	assert.Nil(t, err)
// 	root, err := stage.Commit()
// 	assert.Nil(t, err)

// 	blk := newBlock(parentID, 10, root, devAccounts[0].PrivateKey)
// 	err = repo.AddBlock(blk, tx.Receipts{}, 0)
// 	assert.Nil(t, err)
// 	parentID = blk.Header().ID()

// 	repo.SetBestBlockID(parentID)

// 	err = op.dumpStateLeaves(repo.NewBestChain(), 0, block.Number(parentID)+1)
// 	assert.Nil(t, err)

// 	err = op.pruneTries(repo.NewBestChain(), 0, block.Number(parentID)+1)
// 	assert.Nil(t, err)

// 	closeDB()
// }
