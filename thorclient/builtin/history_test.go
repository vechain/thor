// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	contracts "github.com/vechain/thor/v2/builtin"
)

func TestHistory_BlockID(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	// Mint a few blocks so we have history to read.
	for range 5 {
		require.NoError(t, node.Chain().MintBlock())
	}

	history := NewHistory(client)
	require.Equal(t, contracts.History.Address, history.Address())

	best := node.Chain().Repo().BestBlockSummary().Header.Number()
	target := best - 1

	// Compare against the canonical block ID fetched via the client.
	want, err := client.Block(strconv.FormatUint(uint64(target), 10))
	require.NoError(t, err)

	got, err := history.BlockID(target)
	require.NoError(t, err)
	require.Equal(t, want.ID.Bytes(), got.Bytes())
}

func TestHistory_FutureBlockReverts(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	require.NoError(t, node.Chain().MintBlock())
	require.NoError(t, node.Chain().MintBlock())

	history := NewHistory(client)
	best := node.Chain().Repo().BestBlockSummary().Header.Number()

	// num == block.number must revert (EIP-2935: range is [best-8191, best-1]).
	_, err := history.BlockID(best)
	require.ErrorContains(t, err, "execution reverted")

	_, err = history.BlockID(best + 9999)
	require.ErrorContains(t, err, "execution reverted")
}

func TestHistory_CallRaw_InvalidLength(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	require.NoError(t, node.Chain().MintBlock())

	history := NewHistory(client)

	for _, length := range []int{0, 31, 33} {
		out, err := history.CallRaw(make([]byte, length))
		require.ErrorContains(t, err, "execution reverted", "length=%d", length)
		require.Empty(t, out, "length=%d: EIP-2935 revert must carry no return data", length)
	}
}

func TestHistory_Revision(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	for range 3 {
		require.NoError(t, node.Chain().MintBlock())
	}

	history := NewHistory(client)
	best := node.Chain().Repo().BestBlockSummary().Header.Number()

	// Same query, pinned to the best revision explicitly: must match the default path.
	bestRev := strconv.FormatUint(uint64(best), 10)
	want, err := history.BlockID(best - 1)
	require.NoError(t, err)
	got, err := history.Revision(bestRev).BlockID(best - 1)
	require.NoError(t, err)
	require.Equal(t, want, got)

	// Bad revision string should surface as a request error.
	_, err = history.Revision("not-a-real-revision").BlockID(best - 1)
	require.Error(t, err)
}
