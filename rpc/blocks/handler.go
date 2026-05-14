// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/tx"
)

// Handler implements block query JSON-RPC methods.
type Handler struct {
	repo *chain.Repository
}

// New creates a blocks Handler.
func New(repo *chain.Repository) *Handler {
	return &Handler{repo: repo}
}

// Mount registers all block query methods on the dispatcher.
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_getBlockByHash", h.ethGetBlockByHash)
	s.Register("eth_getBlockByNumber", h.ethGetBlockByNumber)
	s.Register("eth_getBlockTransactionCountByHash", h.ethGetBlockTransactionCountByHash)
	s.Register("eth_getBlockTransactionCountByNumber", h.ethGetBlockTransactionCountByNumber)
	s.Register("eth_getBlockReceipts", h.ethGetBlockReceipts)
	s.Register("eth_getUncleCountByBlockHash", h.ethGetUncleCountByBlockHash)
	s.Register("eth_getUncleCountByBlockNumber", h.ethGetUncleCountByBlockNumber)
	s.Register("eth_getUncleByBlockHashAndIndex", func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, nil) })
	s.Register("eth_getUncleByBlockNumberAndIndex", func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, nil) })
}

func (h *Handler) ethGetBlockByHash(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockQueryParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	return h.getBlockByTag(req.ID, params.Tag, params.FullTxs)
}

func (h *Handler) ethGetBlockByNumber(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockQueryParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	return h.getBlockByTag(req.ID, params.Tag, params.FullTxs)
}

func (h *Handler) getBlockByTag(id json.RawMessage, tag string, fullTxs bool) jsonrpc.Response {
	summary, err := ethconvert.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return jsonrpc.OkResponse(id, nil)
	}
	blk, err := ethconvert.BuildEthBlock(summary.Header, h.repo, fullTxs)
	if err != nil {
		return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
	}
	return jsonrpc.OkResponse(id, blk)
}

func (h *Handler) ethGetBlockTransactionCountByHash(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	return h.txCountByTag(req.ID, params.Tag)
}

func (h *Handler) ethGetBlockTransactionCountByNumber(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	return h.txCountByTag(req.ID, params.Tag)
}

func (h *Handler) txCountByTag(id json.RawMessage, tag string) jsonrpc.Response {
	summary, err := ethconvert.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return jsonrpc.OkResponse(id, nil)
	}
	blk, err := h.repo.GetBlock(summary.Header.ID())
	if err != nil {
		return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
	}
	var count uint64
	for _, t := range blk.Transactions() {
		if t.Type() == tx.TypeEthDynamicFee {
			count++
		}
	}
	return jsonrpc.OkResponse(id, hexutil.Uint64(count))
}

func (h *Handler) ethGetBlockReceipts(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	summary, err := ethconvert.ResolveBlockTag(params.Tag, h.repo)
	if err != nil {
		return jsonrpc.OkResponse(req.ID, nil)
	}
	blk, err := h.repo.GetBlock(summary.Header.ID())
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	receipts, err := h.repo.GetBlockReceipts(summary.Header.ID())
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}

	blockHash := common.Hash(summary.Header.ID())
	blockNum := uint64(summary.Header.Number())
	baseFee := summary.Header.BaseFee()

	ethReceipts := make([]*rpc.EthReceipt, 0)
	for i, t := range blk.Transactions() {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		projIdx := ethconvert.ProjectedEthIndex(receipts, uint64(i))
		cumGas := ethconvert.CumulativeEthGasUsed(receipts, uint64(i))
		logOff := ethconvert.EthLogOffset(receipts, uint64(i))
		ethReceipts = append(ethReceipts, ethconvert.ToEthReceipt(
			t, receipts[i],
			blockHash, blockNum,
			projIdx, cumGas, logOff, baseFee,
		))
	}
	return jsonrpc.OkResponse(req.ID, ethReceipts)
}

func (h *Handler) ethGetUncleCountByBlockHash(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	return h.uncleCountByTag(req.ID, params.Tag)
}

func (h *Handler) ethGetUncleCountByBlockNumber(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	return h.uncleCountByTag(req.ID, params.Tag)
}

func (h *Handler) uncleCountByTag(id json.RawMessage, tag string) jsonrpc.Response {
	if _, err := ethconvert.ResolveBlockTag(tag, h.repo); err != nil {
		return jsonrpc.OkResponse(id, nil)
	}
	return jsonrpc.OkResponse(id, hexutil.Uint64(0))
}
