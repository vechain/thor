// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

// GasLimitSoftLimit is the block gas limit packers settle on: the adaptive
// target is clamped to it, and solo's --gas-limit default equals it.
// Consensus only bounds each block's gas limit to within 1/1024 of its
// parent's, so a higher limit holds only if validators with more than half of
// the stake weight reconfigure. The decode bounds below and logdb's index
// widths (logdb/sequence_test.go) are sized against it; revisit them whenever
// it changes.
const GasLimitSoftLimit uint64 = 40_000_000

// Element-count bounds enforced while RLP-decoding a block body. At
// GasLimitSoftLimit and MaxTxGasLimit each must admit every valid tx or block;
// gas_limits_test.go asserts that.
const (
	// MaxClausesPerTx bounds the clauses decoded from one tx. It equals
	// GasLimitSoftLimit/ClauseGas with no headroom, which is safe: since
	// INTERSTELLAR a tx carries at most MaxTxGasLimit (EIP-7825) gas, i.e. at
	// most 1048 clauses, and every pre-INTERSTELLAR tx on mainnet and testnet
	// fits within it.
	MaxClausesPerTx = 2500

	// MaxTxsPerBlock bounds the txs decoded from one block body. Every tx costs
	// at least TxGas+ClauseGas (21000) intrinsic gas, even with no clauses, so a
	// valid block under GasLimitSoftLimit holds at most 1904 txs.
	MaxTxsPerBlock = 8192

	// MaxClausesPerBlock bounds the clauses decoded from one block body. Every
	// clause costs at least ClauseGas (16000) intrinsic gas, so a valid block
	// under GasLimitSoftLimit holds at most 2500 clauses. Lowering it is unsafe:
	// testnet block 9239749, packed under a 273M gas limit, carries 7761.
	MaxClausesPerBlock = 8192
)
