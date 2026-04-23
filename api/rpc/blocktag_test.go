// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

// fakeCommitter lets tests control what "finalized" and "safe" resolve to.
type fakeCommitter struct {
	finalized thor.Bytes32
	justified thor.Bytes32
	justErr   error
}

func (f *fakeCommitter) Finalized() thor.Bytes32                        { return f.finalized }
func (f *fakeCommitter) Justified() (thor.Bytes32, error)               { return f.justified, f.justErr }
func (f *fakeCommitter) Accepts(thor.Bytes32) (bool, error)             { return true, nil }
func (f *fakeCommitter) Select(*block.Header) (bool, error)             { return true, nil }
func (f *fakeCommitter) CommitBlock(*block.Header, bool) error          { return nil }
func (f *fakeCommitter) ShouldVote(thor.Bytes32) (bool, error)          { return false, nil }

// --- UnmarshalJSON --------------------------------------------------------

func TestBlockTag_Unmarshal_StringTag(t *testing.T) {
	for _, tag := range []string{TagLatest, TagEarliest, TagPending, TagSafe, TagFinalized} {
		t.Run(tag, func(t *testing.T) {
			var b BlockTag
			require.NoError(t, json.Unmarshal([]byte(`"`+tag+`"`), &b))
			assert.Equal(t, tag, b.TagName())
			assert.Nil(t, b.blockHash)
			assert.Nil(t, b.blockNumber)
		})
	}
}

func TestBlockTag_Unmarshal_HexNumber(t *testing.T) {
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"0x1a4"`), &b))
	require.NotNil(t, b.blockNumber)
	assert.Equal(t, uint64(0x1a4), *b.blockNumber)
	assert.Nil(t, b.blockHash)
}

func TestBlockTag_Unmarshal_BareHash(t *testing.T) {
	var b BlockTag
	hex := `"0x` + "aa" + stringRepeat("bb", 31) + `"`
	require.NoError(t, json.Unmarshal([]byte(hex), &b))
	require.NotNil(t, b.blockHash)
	assert.Nil(t, b.blockNumber)
}

func TestBlockTag_Unmarshal_ObjectWithBlockNumber(t *testing.T) {
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`{"blockNumber":"0x2a"}`), &b))
	require.NotNil(t, b.blockNumber)
	assert.Equal(t, uint64(0x2a), *b.blockNumber)
}

func TestBlockTag_Unmarshal_ObjectWithBlockHash_RequireCanonical(t *testing.T) {
	var b BlockTag
	hex := `0x` + "aa" + stringRepeat("11", 31)
	body := `{"blockHash":"` + hex + `","requireCanonical":true}`
	require.NoError(t, json.Unmarshal([]byte(body), &b))
	require.NotNil(t, b.blockHash)
	assert.True(t, b.requireCanonical)
}

func TestBlockTag_Unmarshal_ObjectBothFields_Rejected(t *testing.T) {
	var b BlockTag
	hex := `0x` + stringRepeat("cc", 32)
	body := `{"blockNumber":"0x1","blockHash":"` + hex + `"}`
	err := json.Unmarshal([]byte(body), &b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestBlockTag_Unmarshal_BadShape(t *testing.T) {
	var b BlockTag
	assert.Error(t, json.Unmarshal([]byte(`true`), &b))
	assert.Error(t, json.Unmarshal([]byte(`"malformed"`), &b))
	assert.Error(t, json.Unmarshal([]byte(`"0xzzzz"`), &b))
	assert.Error(t, json.Unmarshal([]byte(`{}`), &b))
}

// --- Resolve --------------------------------------------------------------

func newResolveFixture(t *testing.T) (*testchain.Chain, *fakeCommitter) {
	t.Helper()
	tc, err := testchain.NewWithFork(&thor.ForkConfig{}, 180)
	require.NoError(t, err)

	// Mint one block on top of genesis so tests have more than block 0 to
	// play with. MintBlock takes zero or more txs; zero is fine here.
	require.NoError(t, tc.MintBlock())

	best := tc.Repo().BestBlockSummary().Header.ID()
	return tc, &fakeCommitter{finalized: best, justified: best}
}

func TestBlockTag_Resolve_Latest(t *testing.T) {
	tc, committer := newResolveFixture(t)
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"latest"`), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, tc.Repo().BestBlockSummary().Header.ID(), sum.Header.ID())
}

func TestBlockTag_Resolve_Earliest(t *testing.T) {
	tc, committer := newResolveFixture(t)
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"earliest"`), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), sum.Header.Number())
}

func TestBlockTag_Resolve_HexNumber(t *testing.T) {
	tc, committer := newResolveFixture(t)
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"0x0"`), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), sum.Header.Number())
}

func TestBlockTag_Resolve_HexNumber_OutOfRange(t *testing.T) {
	tc, committer := newResolveFixture(t)
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"0x100000"`), &b))

	_, _, err := b.Resolve(tc.Repo(), committer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBlockTag_Resolve_Finalized(t *testing.T) {
	tc, committer := newResolveFixture(t)
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"finalized"`), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, committer.finalized, sum.Header.ID())
}

func TestBlockTag_Resolve_Safe_FallsBackToFinalized(t *testing.T) {
	tc, committer := newResolveFixture(t)
	committer.justified = thor.Bytes32{} // no justified yet
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"safe"`), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, committer.finalized, sum.Header.ID(), "safe falls back to finalized when no justified checkpoint yet")
}

func TestBlockTag_Resolve_Finalized_NotAvailable(t *testing.T) {
	tc, committer := newResolveFixture(t)
	committer.finalized = thor.Bytes32{}
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(`"finalized"`), &b))

	_, _, err := b.Resolve(tc.Repo(), committer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestBlockTag_Resolve_BlockHash_Canonical(t *testing.T) {
	tc, committer := newResolveFixture(t)
	bestID := tc.Repo().BestBlockSummary().Header.ID()

	body := `"` + bestID.String() + `"`
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(body), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, bestID, sum.Header.ID())
}

func TestBlockTag_Resolve_BlockHash_RequireCanonical_Canonical(t *testing.T) {
	tc, committer := newResolveFixture(t)
	bestID := tc.Repo().BestBlockSummary().Header.ID()

	body := `{"blockHash":"` + bestID.String() + `","requireCanonical":true}`
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(body), &b))

	_, sum, err := b.Resolve(tc.Repo(), committer)
	require.NoError(t, err)
	assert.Equal(t, bestID, sum.Header.ID(), "canonical block with requireCanonical=true resolves normally")
}

// Building a non-canonical sibling is reorg / integration-level setup. Here
// we unit-test that errBlockNotCanonical flows through ToRPCError to the
// canonical JSON-RPC reason — the hard plumbing upstream.
func TestBlockTag_NonCanonical_ErrorMapsToReason(t *testing.T) {
	var h thor.Bytes32
	h[0] = 0xde
	err := errBlockNotCanonical{hash: h}

	var target errBlockNotCanonical
	require.True(t, errors.As(err, &target))
	assert.Equal(t, h, target.hash)

	rpcErr := ToRPCError(err)
	require.NotNil(t, rpcErr)
	assert.Equal(t, CodeServerError, rpcErr.Code)
	data, ok := rpcErr.Data.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, ReasonBlockNotCanonical, data["reason"])
}

func TestBlockTag_Resolve_BlockHash_NotFound(t *testing.T) {
	tc, committer := newResolveFixture(t)
	var h thor.Bytes32
	h[0] = 0xff
	body := `"` + h.String() + `"`
	var b BlockTag
	require.NoError(t, json.Unmarshal([]byte(body), &b))

	_, _, err := b.Resolve(tc.Repo(), committer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- helpers --------------------------------------------------------------

// stringRepeat matches strings.Repeat; local alias avoids an extra import.
func stringRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}
