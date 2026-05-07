// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Handler implements account state JSON-RPC methods.
type Handler struct {
	repo   *chain.Repository
	stater *state.Stater
}

// New creates an accounts Handler.
func New(repo *chain.Repository, stater *state.Stater) *Handler {
	return &Handler{repo: repo, stater: stater}
}

// Mount registers all account state methods on the dispatcher.
func (h *Handler) Mount(s *rpc.Server) {
	s.Register("eth_getBalance", h.ethGetBalance)
	s.Register("eth_getCode", h.ethGetCode)
	s.Register("eth_getStorageAt", h.ethGetStorageAt)
	s.Register("eth_getTransactionCount", h.ethGetTransactionCount)
}

func (h *Handler) ethGetBalance(req rpc.Request) rpc.Response {
	addr, tag, err := parseAddrAndTag(req.Params)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}
	st, err := rpc.StateAt(tag, h.repo, h.stater)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	bal, err := st.GetBalance(addr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(req.ID, (*hexutil.Big)(bal))
}

func (h *Handler) ethGetCode(req rpc.Request) rpc.Response {
	addr, tag, err := parseAddrAndTag(req.Params)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}
	st, err := rpc.StateAt(tag, h.repo, h.stater)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	code, err := st.GetCode(addr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(req.ID, hexutil.Bytes(code))
}

func (h *Handler) ethGetStorageAt(req rpc.Request) rpc.Response {
	var params [3]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [address, slot, blockTag]")
	}
	var addrStr, slotStr, tag string
	if err := json.Unmarshal(params[0], &addrStr); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid address")
	}
	if err := json.Unmarshal(params[1], &slotStr); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid slot")
	}
	if err := json.Unmarshal(params[2], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block tag")
	}

	addr, err := thor.ParseAddress(addrStr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid address")
	}
	slot, err := rpc.ParseBytes32Compact(slotStr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid slot")
	}
	st, err := rpc.StateAt(tag, h.repo, h.stater)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	val, err := st.GetStorage(addr, slot)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(req.ID, common.Hash(val))
}

func (h *Handler) ethGetTransactionCount(req rpc.Request) rpc.Response {
	addr, tag, err := parseAddrAndTag(req.Params)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}
	// NOTE: "pending" returns the confirmed nonce; pool scanning is not implemented.
	st, err := rpc.StateAt(tag, h.repo, h.stater)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	nonce, err := st.GetNonce(addr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(req.ID, hexutil.Uint64(nonce))
}

func parseAddrAndTag(raw json.RawMessage) (thor.Address, string, error) {
	var params [2]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return thor.Address{}, "", fmt.Errorf("expected [address, blockTag]")
	}
	var addrStr, tag string
	if err := json.Unmarshal(params[0], &addrStr); err != nil {
		return thor.Address{}, "", fmt.Errorf("invalid address")
	}
	if err := json.Unmarshal(params[1], &tag); err != nil {
		return thor.Address{}, "", fmt.Errorf("invalid block tag")
	}
	addr, err := thor.ParseAddress(addrStr)
	if err != nil {
		return thor.Address{}, "", fmt.Errorf("invalid address: %w", err)
	}
	return addr, tag, nil
}
