// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/state"
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
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_getBalance", h.ethGetBalance)
	s.Register("eth_getCode", h.ethGetCode)
	s.Register("eth_getStorageAt", h.ethGetStorageAt)
	s.Register("eth_getTransactionCount", h.ethGetTransactionCount)
}

func (h *Handler) ethGetBalance(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.AddressAndTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	st, err := ethconvert.StateAtBlockNumberOrHash(params.Block, h.repo, h.stater)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	bal, err := st.GetBalance(params.Address)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	return jsonrpc.OkResponse(req.ID, (*hexutil.Big)(bal))
}

func (h *Handler) ethGetCode(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.AddressAndTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	st, err := ethconvert.StateAtBlockNumberOrHash(params.Block, h.repo, h.stater)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	code, err := st.GetCode(params.Address)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	return jsonrpc.OkResponse(req.ID, hexutil.Bytes(code))
}

func (h *Handler) ethGetStorageAt(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.StorageAtParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	st, err := ethconvert.StateAtBlockNumberOrHash(params.Block, h.repo, h.stater)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	val, err := st.GetStorage(params.Address, params.Slot)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	return jsonrpc.OkResponse(req.ID, common.Hash(val))
}

func (h *Handler) ethGetTransactionCount(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.AddressAndTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	// TODO "pending" returns the confirmed nonce; pool scanning is not implemented.
	st, err := ethconvert.StateAtBlockNumberOrHash(params.Block, h.repo, h.stater)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	nonce, err := st.GetNonce(params.Address)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	return jsonrpc.OkResponse(req.ID, hexutil.Uint64(nonce))
}
