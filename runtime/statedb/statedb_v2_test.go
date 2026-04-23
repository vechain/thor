// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package statedb

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/muxdb"
	State "github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

// TestV2SetNoncePersists verifies V2 SetNonce writes through to real state
// and is observable by both V1 GetNonce (inherited) and an independent
// state.GetNonce read. V1 SetNonce on the same state remains a no-op.
func TestV2SetNoncePersists(t *testing.T) {
	addr := common.Address{0xbb}
	st := State.New(muxdb.NewMem(), trie.Root{})

	v2 := NewV2(st)
	v2.SetNonce(addr, 3)

	// V2 reads its own write (via inherited V1 GetNonce → real state).
	assert.Equal(t, uint64(3), v2.GetNonce(addr))

	// Independent state read sees the write.
	n, err := st.GetNonce(thor.Address(addr))
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), n)

	// V1 wrapping the same state sees the write on read, and its SetNonce
	// is ignored — V1 never overwrites V2's value.
	v1 := New(st)
	assert.Equal(t, uint64(3), v1.GetNonce(addr))
	v1.SetNonce(addr, 99)
	assert.Equal(t, uint64(3), v1.GetNonce(addr), "V1 SetNonce must be no-op even when V2 wrote earlier")
}

// TestV2InheritsBalance smoke-tests that non-overridden methods reach V1.
// (The whole StateDB surface is thoroughly tested in TestSnapshotRandom; here
// we just confirm the embed wiring works for at least one other method.)
func TestV2InheritsBalance(t *testing.T) {
	addr := common.Address{0xcc}
	st := State.New(muxdb.NewMem(), trie.Root{})
	v2 := NewV2(st)

	v2.AddBalance(addr, big.NewInt(1000))
	assert.Equal(t, 0, v2.GetBalance(addr).Cmp(big.NewInt(1000)))
}
