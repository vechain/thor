// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/tx"
)

// callHistory invokes the EIP-2935 facade with raw 32-byte calldata
// (the EIP-2935 convention has no function selector).
func callHistory(chain *testchain.Chain, num uint32) ([]byte, error) {
	out, _, err := callHistoryWithGas(chain, num)
	return out, err
}

func callHistoryWithGas(chain *testchain.Chain, num uint32) ([]byte, uint64, error) {
	var data [32]byte
	binary.BigEndian.PutUint32(data[28:], num)
	return callHistoryRawWithGas(chain, data[:])
}

// callHistoryRaw invokes the History contract with the given raw calldata
// (no selector mangling). Used for testing invalid-length inputs.
func callHistoryRaw(chain *testchain.Chain, data []byte) ([]byte, error) {
	out, _, err := callHistoryRawWithGas(chain, data)
	return out, err
}

func callHistoryRawWithGas(chain *testchain.Chain, data []byte) ([]byte, uint64, error) {
	addr := builtin.History.Address
	clause := tx.NewClause(&addr).WithData(data)
	trx := new(tx.Builder).
		ChainTag(chain.Repo().ChainTag()).
		Expiration(50).
		Gas(200000).
		Clause(clause).
		Build()
	return chain.ClauseCall(genesis.DevAccounts()[0], trx, 0)
}

func TestHistory_ForkActivation(t *testing.T) {
	chain := newChain(t, nil) // SoloFork: INTERSTELLAR = 1
	require.NoError(t, chain.MintBlock())

	st := chain.State()
	code, err := st.GetCode(builtin.History.Address)
	require.NoError(t, err)
	require.Equal(t, builtin.History.RuntimeBytecodes(), code)
}

func TestHistory_ValidRead(t *testing.T) {
	chain := newChain(t, nil)
	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())

	want, err := chain.Repo().NewBestChain().GetBlockID(1)
	require.NoError(t, err)

	got, err := callHistory(chain, 1)
	require.NoError(t, err)
	require.Equal(t, want.Bytes(), got)
}

// TestHistory_WindowBoundary exercises the edges of the [best-8191, best-1]
// valid range. SERVE_WINDOW is hard-coded to 8191 in history.sol, so the
// chain must be deep enough to expose both sides of the boundary.
func TestHistory_WindowBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("requires minting >8191 blocks; skipped in -short mode")
	}
	chain := newChain(t, nil)
	for range 8193 {
		require.NoError(t, chain.MintBlock())
	}

	best := chain.Repo().BestBlockSummary().Header.Number()

	// Passing num == block.number must revert per EIP-2935 (execution reverted).
	got, err := callHistory(chain, best)
	require.ErrorContains(t, err, "execution reverted")
	require.Empty(t, got, "EIP-2935 revert must carry no return data")

	// distance == 1: the most recent block, the newest in-window value.
	newest := best - 1
	want, err := chain.Repo().NewBestChain().GetBlockID(newest)
	require.NoError(t, err)
	got, err = callHistory(chain, newest)
	require.NoError(t, err)
	require.Equal(t, want.Bytes(), got)

	// distance == 8191: the oldest in-window value (block.number - num == SERVE_WINDOW).
	oldestIn := best - 8191
	want, err = chain.Repo().NewBestChain().GetBlockID(oldestIn)
	require.NoError(t, err)
	got, err = callHistory(chain, oldestIn)
	require.NoError(t, err)
	require.Equal(t, want.Bytes(), got)

	// distance == 8192: one step past the window — must revert with empty data.
	out, err := callHistory(chain, oldestIn-1)
	require.ErrorContains(t, err, "execution reverted")
	require.Empty(t, out, "EIP-2935 revert must carry no return data")
}

func TestHistory_FutureBlockReverts(t *testing.T) {
	chain := newChain(t, nil)
	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())

	best := chain.Repo().BestBlockSummary().Header.Number()
	out, err := callHistory(chain, best+9999)
	require.ErrorContains(t, err, "execution reverted")
	require.Empty(t, out, "EIP-2935 revert must carry no return data")
}

func TestHistory_InvalidCalldataLength(t *testing.T) {
	chain := newChain(t, nil)
	require.NoError(t, chain.MintBlock())

	out, err := callHistoryRaw(chain, make([]byte, 31))
	require.ErrorContains(t, err, "execution reverted")
	require.Empty(t, out, "EIP-2935 revert must carry no return data")

	out, err = callHistoryRaw(chain, make([]byte, 33))
	require.ErrorContains(t, err, "execution reverted")
	require.Empty(t, out, "EIP-2935 revert must carry no return data")

	out, err = callHistoryRaw(chain, nil)
	require.ErrorContains(t, err, "execution reverted")
	require.Empty(t, out, "EIP-2935 revert must carry no return data")
}
