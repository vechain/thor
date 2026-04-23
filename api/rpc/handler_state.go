// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func init() {
	register("eth_getBalance", handleGetBalance)
	register("eth_getCode", handleGetCode)
	register("eth_getStorageAt", handleGetStorageAt)
	register("eth_getTransactionCount", handleGetTransactionCount)
}

// parseAddrAndTag pulls [address, blockTag] from params. blockTag defaults to
// "latest" when the second element is missing — the convention every major
// Ethereum client follows even though EIP-1898 says the tag is required.
func parseAddrAndTag(params json.RawMessage) (thor.Address, BlockTag, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return thor.Address{}, BlockTag{}, InvalidParams("params must be an array")
	}
	if len(args) < 1 || len(args) > 2 {
		return thor.Address{}, BlockTag{}, InvalidParams("expected [address] or [address, blockTag]")
	}
	var addr thor.Address
	if err := json.Unmarshal(args[0], &addr); err != nil {
		return thor.Address{}, BlockTag{}, InvalidParams("address: " + err.Error())
	}
	var tag BlockTag
	if len(args) == 2 && string(args[1]) != "null" {
		if err := json.Unmarshal(args[1], &tag); err != nil {
			return thor.Address{}, BlockTag{}, InvalidParams("blockTag: " + err.Error())
		}
	} else {
		tag = BlockTag{tagName: TagLatest}
	}
	return addr, tag, nil
}

// stateAtTag resolves the tag and returns a read-only *state.State rooted at
// that block.
func stateAtTag(s *Server, tag BlockTag) (*state.State, *RPCError) {
	_, summary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return nil, ToRPCError(err)
	}
	return s.stater.NewState(summary.Root()), nil
}

// handleGetBalance returns VET wei balance for addr at the given block.
func handleGetBalance(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	addr, tag, rerr := parseAddrAndTag(params)
	if rerr != nil {
		return nil, rerr
	}
	st, rerr := stateAtTag(s, tag)
	if rerr != nil {
		return nil, rerr
	}
	bal, err := st.GetBalance(addr)
	if err != nil {
		return nil, InternalError(err)
	}
	return (*hexutil.Big)(bal), nil
}

// handleGetCode returns the contract bytecode at addr. Empty code renders as
// "0x" (hexutil.Bytes default behaviour).
func handleGetCode(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	addr, tag, rerr := parseAddrAndTag(params)
	if rerr != nil {
		return nil, rerr
	}
	st, rerr := stateAtTag(s, tag)
	if rerr != nil {
		return nil, rerr
	}
	code, err := st.GetCode(addr)
	if err != nil {
		return nil, InternalError(err)
	}
	return hexutil.Bytes(code), nil
}

// handleGetStorageAt returns a 32-byte storage slot value at addr for the
// given key. Unset slots render as 32 zero bytes.
func handleGetStorageAt(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParams("params must be an array")
	}
	if len(args) < 2 || len(args) > 3 {
		return nil, InvalidParams("expected [address, key] or [address, key, blockTag]")
	}
	var addr thor.Address
	if err := json.Unmarshal(args[0], &addr); err != nil {
		return nil, InvalidParams("address: " + err.Error())
	}
	var key thor.Bytes32
	if err := json.Unmarshal(args[1], &key); err != nil {
		return nil, InvalidParams("storage key: " + err.Error())
	}
	tag := BlockTag{tagName: TagLatest}
	if len(args) == 3 && string(args[2]) != "null" {
		if err := json.Unmarshal(args[2], &tag); err != nil {
			return nil, InvalidParams("blockTag: " + err.Error())
		}
	}
	st, rerr := stateAtTag(s, tag)
	if rerr != nil {
		return nil, rerr
	}
	v, err := st.GetStorage(addr, key)
	if err != nil {
		return nil, InternalError(err)
	}
	return v, nil
}

// handleGetTransactionCount returns the sequential nonce for addr at the
// given block (spec 3 §5). Pre-INTERSTELLAR blocks never wrote Nonce, so the
// read returns 0 — seamless continuity with the Spec 2 stub. "pending" is
// currently aliased to "latest" (state nonce only; pool's future-nonce queue
// length is Deferred §11.1).
func handleGetTransactionCount(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	addr, tag, rerr := parseAddrAndTag(params)
	if rerr != nil {
		return nil, rerr
	}
	st, rerr := stateAtTag(s, tag)
	if rerr != nil {
		return nil, rerr
	}
	n, err := st.GetNonce(addr)
	if err != nil {
		return nil, InternalError(err)
	}
	return hexutil.Uint64(n), nil
}

// Silence the chain unused-import warning on a go-less build: referenced via
// stateAtTag's use of s.repo (through BlockTag.Resolve), but the Go compiler
// wants a direct dependency for clarity.
var _ = (*chain.Chain)(nil)
