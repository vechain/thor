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

// TestStateDBV2_SetNonceWrites confirms V2 Get/SetNonce hit the underlying
// state, while V1 Get/SetNonce are both stubs that don't touch state.
func TestStateDBV2_SetNonceWrites(t *testing.T) {
	st := State.New(muxdb.NewMem(), trie.Root{})
	addr := common.Address(thor.BytesToAddress([]byte("acc1")))

	v2 := NewV2(st)
	v2.SetNonce(addr, 7)
	assert.Equal(t, uint64(7), v2.GetNonce(addr), "V2 SetNonce must persist")

	// Verify directly on state.
	n, err := st.GetNonce(thor.Address(addr))
	assert.Nil(t, err)
	assert.Equal(t, uint64(7), n)

	// V1 doesn't observe nonces: GetNonce always returns 0, SetNonce is a
	// no-op (state nonce stays at 7 from V2's write).
	v1 := New(st)
	assert.Equal(t, uint64(0), v1.GetNonce(addr), "V1 GetNonce must always return 0")
	v1.SetNonce(addr, 99)
	n, err = st.GetNonce(thor.Address(addr))
	assert.Nil(t, err)
	assert.Equal(t, uint64(7), n, "V1 SetNonce must not change state")
}

// TestStateDBV2_InheritsV1 confirms V2 embeds V1 — non-overridden methods
// (e.g. GetBalance) reach through to V1's implementation.
func TestStateDBV2_InheritsV1(t *testing.T) {
	st := State.New(muxdb.NewMem(), trie.Root{})
	addr := common.Address(thor.BytesToAddress([]byte("acc1")))

	v2 := NewV2(st)
	v2.AddBalance(addr, big.NewInt(1))

	bal := v2.GetBalance(addr)
	assert.Equal(t, int64(1), bal.Int64())
}
