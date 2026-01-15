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
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/test/testchain"

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
	gene, _ := genesis.NewDevnet()
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
	gene, _ := genesis.NewDevnet()
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
	_, err = pruner.awaitUntilFinalized(200000) // Target beyond current best
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

	chain, err := pruner.awaitUntilFinalized(100000)
	assert.Nil(t, err)

	assert.True(t, block.Number(chain.HeadID()) >= 10000)
}

func TestPrune(t *testing.T) {
	db, closeDB, err := newTempFileDB()
	assert.Nil(t, err)

	stater := state.NewStater(db)
	gene, _ := genesis.NewDevnet()
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

func TestGetAfterPrune(t *testing.T) {
	chain, err := testchain.NewDefault()
	assert.NoError(t, err)

	accounts := genesis.DevAccounts()
	to := thor.BytesToAddress([]byte("to"))

	for range 10 {
		err = chain.MintClauses(accounts[0], []*tx.Clause{tx.NewClause(&to).WithValue(big.NewInt(1))})
		assert.NoError(t, err)
	}

	pruner := Pruner{
		repo: chain.Repo(),
		db:   chain.Database(),
	}
	// iterate best chain to 10

	// prune [0, 10)
	err = pruner.pruneTries(chain.Repo().NewBestChain(), 0, chain.Repo().BestBlockSummary().Header.Number())
	assert.NoError(t, err)

	st := chain.State()
	balance, err := st.GetBalance(to)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(10), balance)

	sum, err := chain.Repo().NewBestChain().GetBlockSummary(9)
	assert.NoError(t, err)

	st = state.NewStater(chain.Database()).NewState(sum.Root())
	_, err = st.GetBalance(to)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing trie node")
}

func TestGetStorageRandomlyTouchedAfterPrune(t *testing.T) {
	type testcase struct {
		name           string
		blocks         []int
		expectedEnergy uint64
	}

	var cases = []testcase{
		{
			name:           "touch storage every block",
			blocks:         []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			expectedEnergy: uint64(10),
		},
		{
			name:           "touch only once at 9",
			blocks:         []int{9},
			expectedEnergy: uint64(1),
		},
		{
			name:           "touch only once at 10",
			blocks:         []int{10},
			expectedEnergy: uint64(1),
		},
		{
			name:           "touch randomly before pruning point case 1",
			blocks:         []int{5, 7, 8},
			expectedEnergy: uint64(3),
		},
		{
			name:           "touch randomly before pruning point case 2",
			blocks:         []int{5, 7},
			expectedEnergy: uint64(2),
		},
		{
			name:           "touch randomly before pruning point case 3",
			blocks:         []int{3, 6},
			expectedEnergy: uint64(2),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chain, err := testchain.NewDefault()
			assert.NoError(t, err)

			accounts := genesis.DevAccounts()
			to := thor.BytesToAddress([]byte("to"))

			// prepare energy transfer data
			transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
			assert.True(t, ok)
			transferData, err := transferMethod.EncodeInput(to, big.NewInt(1))
			assert.NoError(t, err)

			for i := range 20 {
				clauses := []*tx.Clause{tx.NewClause(&to).WithValue(big.NewInt(1))}
				if contains(tc.blocks, i+1) {
					// touch energy storage by transferring 1 wei VTHO
					clauses = append(clauses, tx.NewClause(&builtin.Energy.Address).WithData(transferData))
				}

				err = chain.MintClauses(accounts[0], clauses)
				assert.NoError(t, err)
			}

			pruner := Pruner{
				repo: chain.Repo(),
				db:   chain.Database(),
			}
			// iterate best chain to 20

			// prune [0, 10)
			blk10, err := chain.Repo().NewBestChain().GetBlockSummary(10)
			if err != nil {
				t.Fatalf("failed to get block 10: %v", err)
			}
			err = pruner.pruneTries(chain.Repo().NewChain(blk10.Header.ID()), 0, blk10.Header.Number())
			assert.NoError(t, err)

			st := chain.State()
			balance, err := st.GetEnergy(to, chain.Repo().BestBlockSummary().Header.Timestamp(), math.MaxUint64)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedEnergy, balance.Uint64())

			st = state.NewStater(chain.Database()).NewState(blk10.Root())
			_, err = st.GetEnergy(to, blk10.Header.Timestamp(), math.MaxUint64)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedEnergy, balance.Uint64())

			sum, err := chain.Repo().NewBestChain().GetBlockSummary(9)
			assert.NoError(t, err)

			st = state.NewStater(chain.Database()).NewState(sum.Root())
			_, err = st.GetEnergy(to, sum.Header.Timestamp(), math.MaxUint64)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "missing trie node")
		})
	}
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
