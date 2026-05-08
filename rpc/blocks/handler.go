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
	"github.com/vechain/thor/v2/tx"
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
func (h *Handler) Mount(s *rpc.Server) {
	s.Register("eth_getBlockByHash", h.ethGetBlockByHash)
	s.Register("eth_getBlockByNumber", h.ethGetBlockByNumber)
	s.Register("eth_getBlockTransactionCountByHash", h.ethGetBlockTransactionCountByHash)
	s.Register("eth_getBlockTransactionCountByNumber", h.ethGetBlockTransactionCountByNumber)
	s.Register("eth_getBlockReceipts", h.ethGetBlockReceipts)
	s.Register("eth_getUncleCountByBlockHash", h.ethGetUncleCountByBlockHash)
	s.Register("eth_getUncleCountByBlockNumber", h.ethGetUncleCountByBlockNumber)
	s.Register("eth_getUncleByBlockHashAndIndex", func(req rpc.Request) rpc.Response { return rpc.OkResponse(req.ID, nil) })
	s.Register("eth_getUncleByBlockNumberAndIndex", func(req rpc.Request) rpc.Response { return rpc.OkResponse(req.ID, nil) })
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
	return h.getBlockByTag(req.ID, hashStr, fullTxs)
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
	return h.getBlockByTag(req.ID, tag, fullTxs)
}

func (h *Handler) getBlockByTag(id json.RawMessage, tag string, fullTxs bool) rpc.Response {
	summary, err := rpc.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return rpc.OkResponse(id, nil)
	}
	blk, err := rpc.BuildEthBlock(summary.Header, h.repo, h.chainID, fullTxs)
	if err != nil {
		return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
	}
	return rpc.OkResponse(id, blk)
}

func (h *Handler) ethGetBlockTransactionCountByHash(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockHash]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block hash")
	}
	return h.txCountByTag(req.ID, tag)
}

func (h *Handler) ethGetBlockTransactionCountByNumber(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockNumber]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block number or tag")
	}
	return h.txCountByTag(req.ID, tag)
}

func (h *Handler) txCountByTag(id json.RawMessage, tag string) rpc.Response {
	summary, err := rpc.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return rpc.OkResponse(id, nil)
	}
	blk, err := h.repo.GetBlock(summary.Header.ID())
	if err != nil {
		return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
	}
	var count uint64
	for _, t := range blk.Transactions() {
		if t.Type() == tx.TypeEthDynamicFee {
			count++
		}
	}
	return rpc.OkResponse(id, hexutil.Uint64(count))
}

func (h *Handler) ethGetBlockReceipts(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockTag]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block tag")
	}

	summary, err := rpc.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return rpc.OkResponse(req.ID, nil)
	}
	blk, err := h.repo.GetBlock(summary.Header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	receipts, err := h.repo.GetBlockReceipts(summary.Header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	blockHash := common.Hash(summary.Header.ID())
	blockNum := uint64(summary.Header.Number())
	baseFee := summary.Header.BaseFee()

	ethReceipts := make([]*rpc.EthReceipt, 0)
	for i, t := range blk.Transactions() {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		projIdx := rpc.ProjectedEthIndex(receipts, uint64(i))
		cumGas := rpc.CumulativeEthGasUsed(receipts, uint64(i))
		logOff := rpc.EthLogOffset(receipts, uint64(i))
		ethReceipts = append(ethReceipts, rpc.ToEthReceipt(
			t, receipts[i], h.chainID,
			blockHash, blockNum,
			projIdx, cumGas, logOff, baseFee,
		))
	}
	return rpc.OkResponse(req.ID, ethReceipts)
}

func (h *Handler) ethGetUncleCountByBlockHash(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockHash]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block hash")
	}
	return h.uncleCountByTag(req.ID, tag)
}

func (h *Handler) ethGetUncleCountByBlockNumber(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockNumber]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block number or tag")
	}
	return h.uncleCountByTag(req.ID, tag)
}

func (h *Handler) uncleCountByTag(id json.RawMessage, tag string) rpc.Response {
	if _, err := rpc.ResolveBlockTag(tag, h.repo); err != nil {
		return rpc.OkResponse(id, nil)
	}
	return rpc.OkResponse(id, hexutil.Uint64(0))
}
