// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
)

// newReorgTestRepo builds a repository holding b0 <- b1 <- b2, with b2 as the best block,
// plus an unattached b2x that shares b1 as parent. Making b2x the best block reorgs b2 out.
func newReorgTestRepo(t *testing.T) (repo *chain.Repository, b1, b2, b2x *block.Block) {
	t.Helper()

	b0 := new(block.Builder).ParentID(thor.Bytes32{0xff, 0xff, 0xff, 0xff}).Build()
	repo, err := chain.NewRepository(muxdb.NewMem(), b0)
	require.NoError(t, err)

	b1 = signedBlock(t, b0, 10)
	require.NoError(t, repo.AddBlock(b1, nil, 0, true))
	b2 = signedBlock(t, b1, 20)
	require.NoError(t, repo.AddBlock(b2, nil, 0, true))
	b2x = signedBlock(t, b1, 21)

	return repo, b1, b2, b2x
}

func signedBlock(t *testing.T, parent *block.Block, ts uint64) *block.Block {
	t.Helper()

	b := new(block.Builder).
		ParentID(parent.Header().ID()).
		Timestamp(ts).
		Build()
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	sig, err := crypto.Sign(b.Header().SigningHash().Bytes(), pk)
	require.NoError(t, err)
	return b.WithSignature(sig)
}

// A reorg re-emits an already seen block with Obsolete=true. That flag is per emission,
// not a property of the block ID, so it must not be served from the ID keyed message cache.
func TestBeat2Reader_ReadReorg(t *testing.T) {
	repo, b1, b2, b2x := newReorgTestRepo(t)

	reader := newBeat2Reader(repo, b1.Header().ID(), newMessageCache[api.Beat2Message](10))

	msgs, ok, err := reader.Read()
	require.NoError(t, err)
	assert.True(t, ok)
	require.Len(t, msgs, 1)
	canonical := msgs[0].(api.Beat2Message)
	assert.Equal(t, b2.Header().ID(), canonical.ID)
	assert.False(t, canonical.Obsolete)

	// reorg: b2x replaces b2 as the best block
	require.NoError(t, repo.AddBlock(b2x, nil, 1, true))

	msgs, ok, err = reader.Read()
	require.NoError(t, err)
	assert.True(t, ok)
	require.Len(t, msgs, 2)

	rolledBack := msgs[0].(api.Beat2Message)
	assert.Equal(t, b2.Header().ID(), rolledBack.ID)
	assert.True(t, rolledBack.Obsolete, "the re-emission of b2 must carry the rollback signal")
	// the rest of the message is unchanged by the reorg
	assert.Equal(t, canonical.Bloom, rolledBack.Bloom)
	assert.Equal(t, canonical.K, rolledBack.K)

	newHead := msgs[1].(api.Beat2Message)
	assert.Equal(t, b2x.Header().ID(), newHead.ID)
	assert.False(t, newHead.Obsolete)
}

// The cache is shared by every connection, so an emission cached while a block sat on a
// dead branch must not leak the rollback signal to a subscriber reading it as canonical.
func TestBeat2Reader_ReadSharedCacheAfterReorg(t *testing.T) {
	repo, b1, _, b2x := newReorgTestRepo(t)
	require.NoError(t, repo.AddBlock(b2x, nil, 1, false))
	sharedCache := newMessageCache[api.Beat2Message](10)

	// a subscriber positioned on the dead branch sees b2x obsolete
	first := newBeat2Reader(repo, b2x.Header().ID(), sharedCache)
	msgs, _, err := first.Read()
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].(api.Beat2Message).Obsolete)

	// the branch holding b2x then becomes the best chain
	b3x := signedBlock(t, b2x, 31)
	require.NoError(t, repo.AddBlock(b3x, nil, 1, true))

	// a second subscriber reads b2x as canonical and must not be served the cached rollback
	second := newBeat2Reader(repo, b1.Header().ID(), sharedCache)
	msgs, _, err = second.Read()
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	canonical := msgs[0].(api.Beat2Message)
	assert.Equal(t, b2x.Header().ID(), canonical.ID)
	assert.False(t, canonical.Obsolete, "a canonical block must not inherit a cached rollback signal")
}

func TestBeatReader_ReadReorg(t *testing.T) {
	repo, b1, b2, b2x := newReorgTestRepo(t)

	reader := newBeatReader(repo, b1.Header().ID(), newMessageCache[api.BeatMessage](10))

	msgs, _, err := reader.Read()
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].(api.BeatMessage).Obsolete)

	require.NoError(t, repo.AddBlock(b2x, nil, 1, true))

	msgs, _, err = reader.Read()
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	rolledBack := msgs[0].(api.BeatMessage)
	assert.Equal(t, b2.Header().ID(), rolledBack.ID)
	assert.True(t, rolledBack.Obsolete, "the re-emission of b2 must carry the rollback signal")
	assert.False(t, msgs[1].(api.BeatMessage).Obsolete)
}
