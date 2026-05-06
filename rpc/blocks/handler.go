// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"encoding/json"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
)

// Handler implements block query JSON-RPC methods.
type Handler struct {
	repo    *chain.Repository
	chainID uint64
}

// New creates a blocks Handler.
func New(repo *chain.Repository, chainID uint64) *Handler {
	return &Handler{repo: repo, chainID: chainID}
}

// Mount registers all block query methods on the dispatcher.
func (h *Handler) Mount(d *rpc.Dispatcher) {
	d.Register("eth_getBlockByHash", h.ethGetBlockByHash)
	d.Register("eth_getBlockByNumber", h.ethGetBlockByNumber)
}

func (h *Handler) ethGetBlockByHash(req rpc.Request) rpc.Response {
	var params [2]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockHash, fullTransactions]")
	}
	var hashStr string
	if err := json.Unmarshal(params[0], &hashStr); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block hash")
	}
	var fullTxs bool
	if err := json.Unmarshal(params[1], &fullTxs); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid fullTransactions flag")
	}

	summary, err := rpc.ResolveBlockTag(hashStr, h.repo)
	if err != nil {
		return rpc.OkResponse(req.ID, nil)
	}
	blk, err := rpc.BuildEthBlock(summary.Header, h.repo, h.chainID, fullTxs)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(req.ID, blk)
}

func (h *Handler) ethGetBlockByNumber(req rpc.Request) rpc.Response {
	var params [2]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockNumber, fullTransactions]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block number or tag")
	}
	var fullTxs bool
	if err := json.Unmarshal(params[1], &fullTxs); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid fullTransactions flag")
	}

	summary, err := rpc.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return rpc.OkResponse(req.ID, nil)
	}
	blk, err := rpc.BuildEthBlock(summary.Header, h.repo, h.chainID, fullTxs)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(req.ID, blk)
}
