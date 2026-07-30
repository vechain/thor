// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/tx"
)

// Gas per path through the EIP-2935 facade, pinned so changes are deliberate.
// Clause-execution gas only, excluding intrinsic transaction gas. Flat, as the
// VM has no EIP-2929 warm/cold accounting; activating it will move these.
const (
	historyGasSuccess     uint64 = 2085
	historyGasValueSent   uint64 = 45  // value > 0; the callvalue guard runs before any validation
	historyGasBadLength   uint64 = 78  // calldatasize != 32
	historyGasFutureBlock uint64 = 215 // num >= block.number, short-circuits
	historyGasOutOfWindow uint64 = 297 // block.number-num > HISTORY_SERVE_WINDOW
)

// callHistoryRaw invokes History with raw calldata (EIP-2935 has no selector)
// and an optional clause value. A revert lands in res.VMErr with res.GasUsed
// intact.
func callHistoryRaw(t *testing.T, chain *testchain.Chain, data []byte, value *big.Int) *testchain.ClauseResult {
	t.Helper()

	addr := builtin.History.Address
	clause := tx.NewClause(&addr).WithData(data)
	if value != nil {
		clause = clause.WithValue(value)
	}
	trx := new(tx.Builder).
		ChainTag(chain.Repo().ChainTag()).
		Expiration(50).
		Gas(200000).
		Clause(clause).
		Build()

	res, err := chain.ExecClause(genesis.DevAccounts()[0], trx, 0)
	require.NoError(t, err)
	return res
}

// callHistory invokes the facade with a block number as 32-byte calldata.
func callHistory(t *testing.T, chain *testchain.Chain, num uint32) *testchain.ClauseResult {
	t.Helper()

	var data [32]byte
	binary.BigEndian.PutUint32(data[28:], num)
	return callHistoryRaw(t, chain, data[:], nil)
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

	res := callHistory(t, chain, 1)
	require.NoError(t, res.VMErr)
	require.Equal(t, want.Bytes(), res.Data)
	require.Equal(t, historyGasSuccess, res.GasUsed)
}

// TestHistory_WindowBoundary exercises the edges of the [best-8191, best-1]
// valid range. SERVE_WINDOW is hard-coded to 8191 in history.sol, so the
// chain must be deep enough to expose both sides of the boundary.
func TestHistory_WindowBoundary(t *testing.T) {
	chain := newChain(t, nil)
	for range 8193 {
		require.NoError(t, chain.MintBlock())
	}

	best := chain.Repo().BestBlockSummary().Header.Number()

	// Passing num == block.number must revert per EIP-2935.
	res := callHistory(t, chain, best)
	require.ErrorContains(t, res.VMErr, "execution reverted")
	require.Empty(t, res.Data, "EIP-2935 revert must carry no return data")
	require.Equal(t, historyGasFutureBlock, res.GasUsed)

	// distance == 1: the most recent block, the newest in-window value.
	newest := best - 1
	want, err := chain.Repo().NewBestChain().GetBlockID(newest)
	require.NoError(t, err)
	res = callHistory(t, chain, newest)
	require.NoError(t, res.VMErr)
	require.Equal(t, want.Bytes(), res.Data)
	require.Equal(t, historyGasSuccess, res.GasUsed)

	// distance == 8191: the oldest in-window value (block.number - num == SERVE_WINDOW).
	oldestIn := best - 8191
	want, err = chain.Repo().NewBestChain().GetBlockID(oldestIn)
	require.NoError(t, err)
	res = callHistory(t, chain, oldestIn)
	require.NoError(t, res.VMErr)
	require.Equal(t, want.Bytes(), res.Data)
	require.Equal(t, historyGasSuccess, res.GasUsed)

	// distance == 8192: past the window. Clears the num >= block.number check,
	// so it costs more than the future-block revert above.
	res = callHistory(t, chain, oldestIn-1)
	require.ErrorContains(t, res.VMErr, "execution reverted")
	require.Empty(t, res.Data, "EIP-2935 revert must carry no return data")
	require.Equal(t, historyGasOutOfWindow, res.GasUsed)
}

func TestHistory_FutureBlockReverts(t *testing.T) {
	chain := newChain(t, nil)
	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())

	best := chain.Repo().BestBlockSummary().Header.Number()
	res := callHistory(t, chain, best+9999)
	require.ErrorContains(t, res.VMErr, "execution reverted")
	require.Empty(t, res.Data, "EIP-2935 revert must carry no return data")
	require.Equal(t, historyGasFutureBlock, res.GasUsed)
}

func TestHistory_InvalidCalldataLength(t *testing.T) {
	chain := newChain(t, nil)
	require.NoError(t, chain.MintBlock())

	// Every invalid length fails identically. 64 bytes is mainnet's
	// SYSTEM_ADDRESS set-path shape; Thor has no such path.
	for _, data := range [][]byte{nil, make([]byte, 31), make([]byte, 33), make([]byte, 64)} {
		t.Run(fmt.Sprintf("len=%d", len(data)), func(t *testing.T) {
			res := callHistoryRaw(t, chain, data, nil)
			require.ErrorContains(t, res.VMErr, "execution reverted")
			require.Empty(t, res.Data, "EIP-2935 revert must carry no return data")
			require.Equal(t, historyGasBadLength, res.GasUsed)
		})
	}
}

// The fallback is non-payable, a deliberate divergence from the reference
// contract, which has no callvalue guard and would accept the funds. The guard
// runs ahead of every other check, so the gas is identical whatever the
// calldata — pinning it also catches the fallback turning payable by accident.
func TestHistory_RejectsValue(t *testing.T) {
	chain := newChain(t, nil)
	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())

	best := chain.Repo().BestBlockSummary().Header.Number()
	var valid [32]byte
	binary.BigEndian.PutUint32(valid[28:], best-1)

	// Control: the same calldata succeeds when no value is attached.
	res := callHistoryRaw(t, chain, valid[:], nil)
	require.NoError(t, res.VMErr)
	require.Equal(t, historyGasSuccess, res.GasUsed)

	for _, data := range [][]byte{valid[:], make([]byte, 31), nil} {
		t.Run(fmt.Sprintf("len=%d", len(data)), func(t *testing.T) {
			res := callHistoryRaw(t, chain, data, big.NewInt(1))
			require.ErrorContains(t, res.VMErr, "execution reverted")
			require.Empty(t, res.Data, "EIP-2935 revert must carry no return data")
			require.Equal(t, historyGasValueSent, res.GasUsed)
		})
	}
}
