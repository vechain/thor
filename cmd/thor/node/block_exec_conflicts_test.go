// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// A block persisted by repo.AddBlock whose bft.CommitBlock fails must still
// advance maxBlockNum, otherwise a same-height sibling reuses trie version
// (num, 0) and its own root is overwritten.
package node

import (
	"encoding/binary"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// failingCommitter wraps a real bft.Committer and makes CommitBlock fail for
// a chosen header, mimicking a storage error at the point commitBlock calls
// into the bft engine, after the block is already persisted via repo.AddBlock.
// Every other call passes through to the real engine.
type failingCommitter struct {
	bft.Committer
	failFor thor.Bytes32
}

func (f *failingCommitter) CommitBlock(h *block.Header, conflicts uint32, packing bool) error {
	if h.ID() == f.failFor {
		return errors.New("injected commit failure")
	}
	return f.Committer.CommitBlock(h, conflicts, packing)
}

// conflictsTestSetup builds a minimal single-proposer chain with all forks
// (including FINALITY) active from genesis, plus a real bft.Engine.
type conflictsTestSetup struct {
	repo     *chain.Repository
	stater   *state.Stater
	fc       *thor.ForkConfig
	engine   *bft.Engine
	proposer genesis.DevAccount
}

func newConflictsTestSetup(t *testing.T) *conflictsTestSetup {
	t.Helper()
	fc := testchain.DefaultForkConfig // thor.ForkConfig{}: every fork, including FINALITY, at block 0
	// Launch well in the past so packing several blocks in a row doesn't trip
	// consensus's "block in the future" tolerance (nowTimestamp + BlockInterval).
	now := uint64(time.Now().Unix())
	launchTime := now - now%thor.BlockInterval() - 100*thor.BlockInterval()
	gene, err := testchain.CreateGenesis(genesis.DevConfig{ForkConfig: &fc, LaunchTime: launchTime}, 1, 180, 180)
	require.NoError(t, err)

	db := muxdb.NewMem()
	stater := state.NewStater(db)
	geneBlk, _, _, err := gene.Build(stater)
	require.NoError(t, err)

	repo, err := chain.NewRepository(db, geneBlk)
	require.NoError(t, err)

	proposer := genesis.DevAccounts()[0]
	engine, err := bft.NewEngine(repo, db, &fc, proposer.Address)
	require.NoError(t, err)

	return &conflictsTestSetup{repo: repo, stater: stater, fc: &fc, engine: engine, proposer: proposer}
}

func (s *conflictsTestSetup) node(t *testing.T, committer bft.Committer) *Node {
	t.Helper()
	memLogdb, err := logdb.NewMem()
	require.NoError(t, err)
	master := &Master{PrivateKey: s.proposer.PrivateKey}
	return New(
		master, s.repo, committer, s.stater, memLogdb, &mockTxPool{},
		t.TempDir(), &mockCommunicator{peerCount: 1}, s.fc,
		Options{SkipLogs: true, TargetGasLimit: 10_000_000},
		consensus.New(s.repo, s.stater, s.fc),
		packer.New(s.repo, s.stater, master.Address(), master.Beneficiary, s.fc, 10_000_000),
	)
}

// packOn packs and signs a block on top of parent, carrying a value transfer
// of amount to `to` so its state root (and block ID) differs from any other
// block built on the same parent.
func (s *conflictsTestSetup) packOn(t *testing.T, parent *chain.BlockSummary, to thor.Address, amount int64) *block.Block {
	t.Helper()
	p := packer.New(s.repo, s.stater, s.proposer.Address, nil, s.fc, 0)
	flow, err := p.Schedule(parent, parent.Header.Timestamp()+thor.BlockInterval())
	require.NoError(t, err)

	trx := tx.MustSign(new(tx.Builder).
		ChainTag(s.repo.ChainTag()).
		BlockRef(tx.NewBlockRef(0)).
		Expiration(100).
		Gas(21_000).
		Nonce(datagen.RandUint64()).
		Clause(tx.NewClause(&to).WithValue(big.NewInt(amount))).
		Build(), genesis.DevAccounts()[1].PrivateKey)
	require.NoError(t, flow.Adopt(trx))

	blk, _, _, err := flow.Pack(s.proposer.PrivateKey, 0, false)
	require.NoError(t, err)
	return blk
}

// TestCommitBlock_AdvancesMaxBlockNumAtPersistence drives the fixed
// invariant end to end: A is processed first with its bft.CommitBlock made
// to fail after repo.AddBlock already persisted it. maxBlockNum must still
// advance so that A's own child is accepted and a same-height sibling B is
// reconciled via ScanConflicts instead of reusing A's trie version.
func TestCommitBlock_AdvancesMaxBlockNumAtPersistence(t *testing.T) {
	setup := newConflictsTestSetup(t)

	addrX := thor.BytesToAddress([]byte("addr-X"))
	addrY := thor.BytesToAddress([]byte("addr-Y"))
	genesisSummary := setup.repo.BestBlockSummary()
	blockA := setup.packOn(t, genesisSummary, addrX, 111)
	blockB := setup.packOn(t, genesisSummary, addrY, 222)

	require.Equal(t, blockA.Header().ParentID(), blockB.Header().ParentID())
	require.NotEqual(t, blockA.Header().ID(), blockB.Header().ID())

	failing := &failingCommitter{Committer: setup.engine, failFor: blockA.Header().ID()}
	node := setup.node(t, failing)
	maxNum, err := setup.repo.GetMaxBlockNum()
	require.NoError(t, err)
	node.maxBlockNum = maxNum

	// A is persisted via repo.AddBlock, then bft.CommitBlock is made to fail.
	_, err = node.processBlock(blockA, new(blockStats))
	require.ErrorContains(t, err, "bft commits")

	confAtHeight, err := setup.repo.ScanConflicts(1)
	require.NoError(t, err)
	require.Equal(t, uint32(1), confAtHeight, "A must be persisted despite the CommitBlock error")
	require.Equal(t, uint32(1), node.maxBlockNum, "maxBlockNum must advance once the block is persisted")

	// A's own child is accepted through the fast path, not rejected as temporarily unprocessable.
	sumA, err := setup.repo.GetBlockSummary(blockA.Header().ID())
	require.NoError(t, err)
	blockChild := setup.packOn(t, sumA, addrX, 1)
	_, err = node.processBlock(blockChild, new(blockStats))
	require.NoError(t, err)
	require.Equal(t, uint32(2), node.maxBlockNum)

	// B, a same-height sibling of A, is reconciled via ScanConflicts instead of taking the fast path.
	_, err = node.processBlock(blockB, new(blockStats))
	require.NoError(t, err)

	sumA2, err := setup.repo.GetBlockSummary(blockA.Header().ID())
	require.NoError(t, err)
	sumB, err := setup.repo.GetBlockSummary(blockB.Header().ID())
	require.NoError(t, err)
	require.Equal(t, uint32(0), sumA2.Conflicts)
	require.Equal(t, uint32(1), sumB.Conflicts)

	balXviaA, err := setup.stater.NewState(sumA2.Root()).GetBalance(addrX)
	require.NoError(t, err)
	require.Equal(t, 0, big.NewInt(111).Cmp(balXviaA), "A's own state root must still serve A's balance(X)=111")

	gotID, err := setup.repo.NewChain(blockA.Header().ID()).GetBlockID(1)
	require.NoError(t, err)
	require.Equal(t, blockA.Header().ID(), gotID, "chain rooted at A must resolve its own height to itself")
}

// TestCommitBlock_NoFailure is the control group: without a failed
// CommitBlock, maxBlockNum advances after A, so B goes through the
// ScanConflicts path and gets conflicts=1.
func TestCommitBlock_NoFailure(t *testing.T) {
	setup := newConflictsTestSetup(t)

	addrX := thor.BytesToAddress([]byte("addr-X"))
	addrY := thor.BytesToAddress([]byte("addr-Y"))
	genesisSummary := setup.repo.BestBlockSummary()
	blockA := setup.packOn(t, genesisSummary, addrX, 111)
	blockB := setup.packOn(t, genesisSummary, addrY, 222)

	node := setup.node(t, setup.engine) // no failure injected
	maxNum, err := setup.repo.GetMaxBlockNum()
	require.NoError(t, err)
	node.maxBlockNum = maxNum

	_, err = node.processBlock(blockA, new(blockStats))
	require.NoError(t, err)
	require.Equal(t, uint32(1), node.maxBlockNum, "maxBlockNum advances on success")

	_, err = node.processBlock(blockB, new(blockStats))
	require.NoError(t, err)

	sumA, err := setup.repo.GetBlockSummary(blockA.Header().ID())
	require.NoError(t, err)
	sumB, err := setup.repo.GetBlockSummary(blockB.Header().ID())
	require.NoError(t, err)
	require.Equal(t, uint32(1), sumB.Conflicts, "B goes through ScanConflicts and gets conflicts=1")

	balXviaA, err := setup.stater.NewState(sumA.Root()).GetBalance(addrX)
	require.NoError(t, err)
	require.Equal(t, 0, big.NewInt(111).Cmp(balXviaA), "A's own state is intact, not overwritten by B")

	gotID, err := setup.repo.NewChain(blockA.Header().ID()).GetBlockID(1)
	require.NoError(t, err)
	require.Equal(t, blockA.Header().ID(), gotID, "chain rooted at A resolves its own height to itself")
}

// TestGuardBlockProcessing_FailedBeforePersistNotAdvance pins #1049: a block
// that fails before it ever reaches repo.AddBlock must not advance
// maxBlockNum, otherwise a stream of invalid blocks could push maxBlockNum
// arbitrarily far ahead and let far-future garbage skip the cheap
// errBlockTemporaryUnprocessable rejection.
func TestGuardBlockProcessing_FailedBeforePersistNotAdvance(t *testing.T) {
	setup := newConflictsTestSetup(t)
	node := setup.node(t, setup.engine)
	maxNum, err := setup.repo.GetMaxBlockNum()
	require.NoError(t, err)
	node.maxBlockNum = maxNum

	// A block referencing a parent that doesn't exist in the repository fails
	// consensus/lookup before repo.AddBlock is ever reached.
	var fakeParentID thor.Bytes32
	binary.BigEndian.PutUint32(fakeParentID[:], maxNum)
	copy(fakeParentID[4:], datagen.RandomHash().Bytes())
	badBlock := new(block.Builder).
		ParentID(fakeParentID).
		Timestamp(setup.repo.BestBlockSummary().Header.Timestamp() + thor.BlockInterval()).
		Build()

	_, err = node.processBlock(badBlock, new(blockStats))
	require.ErrorIs(t, err, errParentMissing)
	require.Equal(t, maxNum, node.maxBlockNum, "maxBlockNum must not advance for a block that never reached AddBlock")
}
