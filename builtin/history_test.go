// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
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

// TestHistory_NotDeployedBeforeFork is the negative half of
// TestHistory_ForkActivation: runtime.New only deploys the facade on the block
// that equals forkConfig.INTERSTELLAR, so with the fork disabled the address
// must stay bare.
func TestHistory_NotDeployedBeforeFork(t *testing.T) {
	fc := thor.SoloFork
	fc.INTERSTELLAR = math.MaxUint32 // never activates
	chain := newChain(t, &fc)
	require.NoError(t, chain.MintBlock())
	require.NoError(t, chain.MintBlock())

	code, err := chain.State().GetCode(builtin.History.Address)
	require.NoError(t, err)
	require.Empty(t, code, "History must not be deployed before INTERSTELLAR")

	// Worth pinning because it is a trap for integrators: calling a codeless
	// address is a plain transfer, so a pre-fork chain answers with success and
	// empty data rather than the revert every post-fork failure path gives.
	best := chain.Repo().BestBlockSummary().Header.Number()
	res := callHistory(t, chain, best-1)
	require.NoError(t, res.VMErr)
	require.Empty(t, res.Data)
}

// history.sol reaches the Extension builtin through a hardcoded Solidity
// address constant, and that address is derived from the Go contract's *name*
// (thor.BytesToAddress([]byte("Extension"))). Renaming the builtin would move
// the address and leave the facade calling into an empty account — which fails
// open, returning zero rather than reverting. Nothing else in the suite would
// catch it, hence this pin.
func TestHistory_ExtensionAddressPin(t *testing.T) {
	require.Equal(t,
		"0x0000000000000000000000457874656e73696f6e",
		builtin.Extension.Address.String(),
		"EXTENSION constant in builtin/gen/history.sol no longer matches builtin.Extension.Address",
	)
}

// blockhashProbe returns the init code of a contract whose runtime is:
//
//	60 00  PUSH1 0        60 00  PUSH1 0
//	35     CALLDATALOAD   52     MSTORE
//	40     BLOCKHASH      60 20  PUSH1 32
//	                      60 00  PUSH1 0
//	                      f3     RETURN
//
// i.e. `return blockhash(calldataload(0))`. Hand-assembled rather than added to
// builtin/gen so a test fixture never lands in the builtin contract set.
func blockhashProbe() []byte {
	const runtime = "6000354060005260206000f3" // 12 bytes
	const deploy = "600c600c600039600c6000f3"  // CODECOPY runtime from offset 12, return it
	return hexutil.MustDecode("0x" + deploy + runtime)
}

// TestHistory_MatchesBlockhashOpcode is the EIP-2935 equivalence claim stated as
// a test: inside BLOCKHASH's 256-block window the facade and the opcode must
// agree exactly, and past it the facade keeps serving while the opcode goes
// silent. That gap up to 8191 blocks is the entire point of the EIP, so pin
// both halves.
func TestHistory_MatchesBlockhashOpcode(t *testing.T) {
	chain := newChain(t, nil)
	acc := genesis.DevAccounts()[0]

	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{tx.NewClause(nil).WithData(blockhashProbe())}))
	blk, err := chain.BestBlock()
	require.NoError(t, err)
	require.Len(t, blk.Transactions(), 1)

	deployID := blk.Transactions()[0].ID()
	receipt, err := chain.GetTxReceipt(deployID)
	require.NoError(t, err)
	require.False(t, receipt.Reverted, "probe deployment reverted")
	probe := thor.CreateContractAddress(deployID, 0, 0)

	// Deep enough that best-257 falls outside BLOCKHASH's window but inside the
	// facade's 8191-block one.
	for range 300 {
		require.NoError(t, chain.MintBlock())
	}
	best := chain.Repo().BestBlockSummary().Header.Number()
	bestChain := chain.Repo().NewBestChain()

	callProbe := func(num uint32) []byte {
		t.Helper()
		var data [32]byte
		binary.BigEndian.PutUint32(data[28:], num)
		clause := tx.NewClause(&probe).WithData(data[:])
		trx := new(tx.Builder).
			ChainTag(chain.Repo().ChainTag()).
			Expiration(50).
			Gas(200000).
			Clause(clause).
			Build()
		res, err := chain.ExecClause(acc, trx, 0)
		require.NoError(t, err)
		require.NoError(t, res.VMErr)
		return res.Data
	}

	// opBlockhash serves [best-256, best-1]; both edges must match the facade.
	for _, num := range []uint32{best - 1, best - 256} {
		t.Run(fmt.Sprintf("distance=%d", best-num), func(t *testing.T) {
			want, err := bestChain.GetBlockID(num)
			require.NoError(t, err)

			res := callHistory(t, chain, num)
			require.NoError(t, res.VMErr)
			require.Equal(t, want.Bytes(), res.Data, "facade disagrees with the canonical block ID")
			require.Equal(t, want.Bytes(), callProbe(num), "BLOCKHASH disagrees with the canonical block ID")
		})
	}

	// distance == 257: past BLOCKHASH's window, still inside the facade's.
	t.Run("distance=257", func(t *testing.T) {
		num := best - 257
		want, err := bestChain.GetBlockID(num)
		require.NoError(t, err)

		res := callHistory(t, chain, num)
		require.NoError(t, res.VMErr)
		require.Equal(t, want.Bytes(), res.Data, "facade must serve beyond 256 blocks")
		require.Equal(t, make([]byte, 32), callProbe(num), "BLOCKHASH must return zero beyond 256 blocks")
	})
}
