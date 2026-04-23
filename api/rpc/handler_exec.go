// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

func init() {
	register("eth_call", handleCall)
	register("eth_estimateGas", handleEstimateGas)
}

// ethCallArgs is the eth_call / eth_estimateGas request shape.
//
// `data` and `input` are synonyms per the Ethereum JSON-RPC spec — wallets
// produce either depending on vintage. We accept both but reject the case
// where both are supplied with different bytes. `gasPrice` and `maxFeePerGas`
// are mutually exclusive (a tx can't be simultaneously legacy and EIP-1559).
type ethCallArgs struct {
	From                 *thor.Address `json:"from"`
	To                   *thor.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Data                 *hexutil.Bytes  `json:"data"`
	Input                *hexutil.Bytes  `json:"input"`
	AccessList           []any           `json:"accessList"`
	StateOverrides       json.RawMessage `json:"stateOverrides"`
}

// resolveCallData collapses the data / input synonyms into a single byte slice.
func (a *ethCallArgs) resolveCallData() ([]byte, *RPCError) {
	switch {
	case a.Data == nil && a.Input == nil:
		return nil, nil
	case a.Data != nil && a.Input == nil:
		return []byte(*a.Data), nil
	case a.Data == nil && a.Input != nil:
		return []byte(*a.Input), nil
	default:
		// Both set — must match.
		if string(*a.Data) != string(*a.Input) {
			return nil, InvalidParams("data and input fields must match when both are present")
		}
		return []byte(*a.Data), nil
	}
}

// handleCall runs an eth_call: read-only invocation that returns the raw EVM
// output bytes. access_list_not_supported / state_overrides_not_supported /
// gas_cap_exceeded surface as reason-coded server errors; VM reverts surface
// as execution_reverted with the decoded ABI reason in the message.
func handleCall(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	args, tag, rerr := parseExecParams(params)
	if rerr != nil {
		return nil, rerr
	}

	output, rerr := runEthCall(s, args, tag)
	if rerr != nil {
		return nil, rerr
	}
	if output.vmErr != nil {
		return nil, ReasonError(ReasonExecutionReverted, decodeRevertReason(output.data, output.vmErr))
	}
	return hexutil.Bytes(output.data), nil
}

// handleEstimateGas runs a binary search for the minimum gas that lets the
// call execute without OOG / revert. Semantics mirror go-ethereum's estimate
// within Thor's gas envelope.
func handleEstimateGas(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	args, tag, rerr := parseExecParams(params)
	if rerr != nil {
		return nil, rerr
	}

	// Upper bound: user-supplied gas or the server-wide cap.
	hi := uint64(s.cfg.CallGasLimit)
	if hi == 0 {
		hi = 50_000_000 // safety default when config is bare
	}
	if args.Gas != nil && uint64(*args.Gas) < hi {
		hi = uint64(*args.Gas)
	}
	// Lower bound: 21k (intrinsic gas for a plain transfer). The EVM will
	// floor at intrinsic gas inside the runtime too.
	lo := uint64(21_000)
	if hi < lo {
		lo = hi
	}

	// Probe at hi first — if that doesn't succeed, estimation is impossible.
	argsCopy := *args
	gas := hexutil.Uint64(hi)
	argsCopy.Gas = &gas

	output, rerr := runEthCall(s, &argsCopy, tag)
	if rerr != nil {
		return nil, rerr
	}
	if output.vmErr != nil {
		return nil, ReasonError(ReasonExecutionReverted, decodeRevertReason(output.data, output.vmErr))
	}

	// Binary-search the minimal gas that succeeds.
	for lo+1 < hi {
		mid := (lo + hi) / 2
		midGas := hexutil.Uint64(mid)
		argsCopy.Gas = &midGas
		out, rerr := runEthCall(s, &argsCopy, tag)
		if rerr != nil {
			return nil, rerr
		}
		if out.vmErr != nil {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hexutil.Uint64(hi), nil
}

// --- shared execution pipeline -------------------------------------------

type callResult struct {
	data  []byte
	vmErr error
}

// parseExecParams unpacks the [args, blockTag] shape and enforces the reason-
// coded input rules (access list, state overrides, gas cap).
func parseExecParams(params json.RawMessage) (*ethCallArgs, BlockTag, *RPCError) {
	var raw []json.RawMessage
	if err := json.Unmarshal(params, &raw); err != nil {
		return nil, BlockTag{}, InvalidParams("params must be an array")
	}
	if len(raw) < 1 || len(raw) > 2 {
		return nil, BlockTag{}, InvalidParams("expected [callArgs] or [callArgs, blockTag]")
	}
	var args ethCallArgs
	if err := json.Unmarshal(raw[0], &args); err != nil {
		return nil, BlockTag{}, InvalidParams("callArgs: " + err.Error())
	}
	tag := BlockTag{tagName: TagLatest}
	if len(raw) == 2 && string(raw[1]) != "null" {
		if err := json.Unmarshal(raw[1], &tag); err != nil {
			return nil, BlockTag{}, InvalidParams("blockTag: " + err.Error())
		}
	}

	// Spec §3 D3 rejections — both have dedicated reasons.
	if len(args.AccessList) > 0 {
		return nil, BlockTag{}, ReasonError(ReasonAccessListNotSupported, "eth_call / eth_estimateGas do not accept access lists on this node")
	}
	if len(args.StateOverrides) > 0 && string(args.StateOverrides) != "null" && string(args.StateOverrides) != "{}" {
		return nil, BlockTag{}, ReasonError(ReasonStateOverridesNotSupported, "eth_call / eth_estimateGas do not accept stateOverrides on this node")
	}

	// Can't mix gasPrice and maxFeePerGas.
	if args.GasPrice != nil && args.MaxFeePerGas != nil {
		return nil, BlockTag{}, InvalidParams("gasPrice and maxFeePerGas are mutually exclusive")
	}

	return &args, tag, nil
}

// runEthCall executes the call via runtime.New on a state checked out at the
// block the tag points to, and returns the raw EVM output + vmErr. Non-VM
// errors (state lookup, repo) are bubbled up as InternalError / reason.
func runEthCall(s *Server, args *ethCallArgs, tag BlockTag) (*callResult, *RPCError) {
	_, summary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return nil, ToRPCError(err)
	}
	st := s.stater.NewState(summary.Root())

	data, rerr := args.resolveCallData()
	if rerr != nil {
		return nil, rerr
	}

	// Enforce gas cap.
	gas := s.cfg.CallGasLimit
	if gas == 0 {
		gas = 50_000_000
	}
	if args.Gas != nil {
		if uint64(*args.Gas) > gas {
			return nil, ReasonError(ReasonGasCapExceeded, fmt.Sprintf("gas %d exceeds cap %d", uint64(*args.Gas), gas))
		}
		gas = uint64(*args.Gas)
	}

	var from thor.Address
	if args.From != nil {
		from = *args.From
	}

	var value *big.Int
	if args.Value != nil {
		value = (*big.Int)(args.Value)
	} else {
		value = new(big.Int)
	}

	gasPrice := new(big.Int)
	if args.GasPrice != nil {
		gasPrice = (*big.Int)(args.GasPrice)
	} else if args.MaxFeePerGas != nil {
		gasPrice = (*big.Int)(args.MaxFeePerGas)
	}

	clause := tx.NewClause(args.To).WithValue(value).WithData(data)

	signer, _ := summary.Header.Signer()
	rt := runtime.New(
		s.repo.NewChain(summary.Header.ParentID()),
		st,
		&xenv.BlockContext{
			Beneficiary: summary.Header.Beneficiary(),
			Signer:      signer,
			Number:      summary.Header.Number(),
			Time:        summary.Header.Timestamp(),
			GasLimit:    summary.Header.GasLimit(),
			TotalScore:  summary.Header.TotalScore(),
			BaseFee:     summary.Header.BaseFee(),
		},
		s.forkConfig,
	)
	txCtx := &xenv.TransactionContext{
		Origin:      from,
		GasPrice:    gasPrice,
		ClauseCount: 1,
	}

	exec, _ := rt.PrepareClause(clause, 0, gas, txCtx)
	out, _, execErr := exec()
	if execErr != nil {
		return nil, InternalError(execErr)
	}
	return &callResult{data: out.Data, vmErr: out.VMErr}, nil
}

// decodeRevertReason extracts the ABI-encoded revert reason from the raw
// return data when present (standard Error(string) selector 0x08c379a0).
// Falls back to the vmErr string when the payload is opaque.
func decodeRevertReason(data []byte, vmErr error) string {
	if len(data) >= 4 {
		// Error(string) selector
		if data[0] == 0x08 && data[1] == 0xc3 && data[2] == 0x79 && data[3] == 0xa0 {
			if msg, ok := abiDecodeString(data[4:]); ok {
				return "execution reverted: " + msg
			}
		}
		// Panic(uint256) selector
		if data[0] == 0x4e && data[1] == 0x48 && data[2] == 0x7b && data[3] == 0x71 {
			return "execution reverted: panic"
		}
	}
	if vmErr != nil {
		return "execution reverted: " + vmErr.Error()
	}
	return "execution reverted"
}

// abiDecodeString parses an ABI-encoded dynamic string (offset + length +
// bytes). Returns ("", false) on malformed input; callers fall back to the
// vmErr message.
func abiDecodeString(payload []byte) (string, bool) {
	if len(payload) < 64 {
		return "", false
	}
	// payload[0..32] = offset (must be 0x20 for a single string arg)
	offset := new(big.Int).SetBytes(payload[:32]).Uint64()
	if offset+32 > uint64(len(payload)) {
		return "", false
	}
	strLen := new(big.Int).SetBytes(payload[offset : offset+32]).Uint64()
	start := offset + 32
	if start+strLen > uint64(len(payload)) {
		return "", false
	}
	return string(payload[start : start+strLen]), true
}

// Unused local references to keep chain/block/state/runtime imports honest
// (they're referenced inside runEthCall; the var below silences lint if a
// future refactor hoists those into a helper that temporarily drops the
// direct symbol use).
var (
	_ = (*chain.Chain)(nil)
	_ = (*block.Block)(nil)
	_ = (*state.State)(nil)
	_ = runtime.New
)
