// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// Handler implements transaction JSON-RPC methods.
type Handler struct {
	repo    *chain.Repository
	chainID uint64
	txPool  txpool.Pool
}

// New creates a transactions Handler.
func New(repo *chain.Repository, chainID uint64, txPool txpool.Pool) *Handler {
	return &Handler{repo: repo, chainID: chainID, txPool: txPool}
}

// Mount registers all transaction methods on the dispatcher.
func (h *Handler) Mount(d *rpc.Dispatcher) {
	d.Register("eth_getTransactionByHash", h.ethGetTransactionByHash)
	d.Register("eth_getTransactionByBlockHashAndIndex", h.ethGetTransactionByBlockHashAndIndex)
	d.Register("eth_getTransactionByBlockNumberAndIndex", h.ethGetTransactionByBlockNumberAndIndex)
	d.Register("eth_getTransactionReceipt", h.ethGetTransactionReceipt)
	d.Register("eth_sendRawTransaction", h.ethSendRawTransaction)
}

func (h *Handler) ethGetTransactionByHash(req rpc.Request) rpc.Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [txHash]")
	}
	id, err := rpc.ParseThorBytes32(params[0])
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid tx hash")
	}

	bestChain := h.repo.NewBestChain()
	t, meta, err := bestChain.GetTransaction(id)
	if err != nil || t.Type() != tx.TypeEthTyped1559 {
		return rpc.OkResponse(req.ID, nil)
	}

	header, err := bestChain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	receipts, err := h.repo.GetBlockReceipts(header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	projIdx := rpc.ProjectedEthIndex(receipts, meta.Index)
	return rpc.OkResponse(req.ID, rpc.ToEthTx(t, h.chainID, common.Hash(header.ID()), uint64(header.Number()), projIdx, header.BaseFee()))
}

func (h *Handler) ethGetTransactionByBlockHashAndIndex(req rpc.Request) rpc.Response {
	var params [2]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockHash, index]")
	}
	var hashStr string
	if err := json.Unmarshal(params[0], &hashStr); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block hash")
	}
	var idxStr string
	if err := json.Unmarshal(params[1], &idxStr); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid index")
	}

	summary, err := rpc.ResolveBlockTag(hashStr, h.repo)
	if err != nil {
		return rpc.OkResponse(req.ID, nil)
	}
	return h.txByBlockAndEthIndex(req, summary.Header, idxStr)
}

func (h *Handler) ethGetTransactionByBlockNumberAndIndex(req rpc.Request) rpc.Response {
	var params [2]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockNumber, index]")
	}
	var tag string
	if err := json.Unmarshal(params[0], &tag); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block number or tag")
	}
	var idxStr string
	if err := json.Unmarshal(params[1], &idxStr); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid index")
	}

	summary, err := rpc.ResolveBlockTag(tag, h.repo)
	if err != nil {
		return rpc.OkResponse(req.ID, nil)
	}
	return h.txByBlockAndEthIndex(req, summary.Header, idxStr)
}

func (h *Handler) txByBlockAndEthIndex(req rpc.Request, header *block.Header, idxStr string) rpc.Response {
	ethIdx, err := rpc.ParseHexUint64(idxStr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid index")
	}

	blk, err := h.repo.GetBlock(header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	receipts, err := h.repo.GetBlockReceipts(header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	blockHash := common.Hash(header.ID())
	blockNum := uint64(header.Number())
	var projIdx uint64

	for i, t := range blk.Transactions() {
		if t.Type() != tx.TypeEthTyped1559 {
			continue
		}
		if projIdx == ethIdx {
			return rpc.OkResponse(req.ID, rpc.ToEthTx(t, h.chainID, blockHash, blockNum, projIdx, header.BaseFee()))
		}
		_ = receipts[i] // bounds check
		projIdx++
	}
	return rpc.OkResponse(req.ID, nil)
}

func (h *Handler) ethGetTransactionReceipt(req rpc.Request) rpc.Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [txHash]")
	}
	id, err := rpc.ParseThorBytes32(params[0])
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid tx hash")
	}

	bestChain := h.repo.NewBestChain()
	t, meta, err := bestChain.GetTransaction(id)
	if err != nil || t.Type() != tx.TypeEthTyped1559 {
		return rpc.OkResponse(req.ID, nil)
	}

	header, err := bestChain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	receipts, err := h.repo.GetBlockReceipts(header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	receipt := receipts[meta.Index]
	projIdx := rpc.ProjectedEthIndex(receipts, meta.Index)
	cumGas := rpc.CumulativeEthGasUsed(receipts, meta.Index)
	logOff := rpc.EthLogOffset(receipts, meta.Index)

	return rpc.OkResponse(req.ID, rpc.ToEthReceipt(
		t, receipt, h.chainID,
		common.Hash(header.ID()), uint64(header.Number()),
		projIdx, cumGas, logOff, header.BaseFee(),
	))
}

func (h *Handler) ethSendRawTransaction(req rpc.Request) rpc.Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [rawTx]")
	}
	raw, err := hexutil.Decode(params[0])
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid hex encoding")
	}

	parsed, err := tx.ParseEthTransaction(raw, h.chainID)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeServerError, err.Error())
	}
	if err := h.txPool.AddLocal(parsed); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeServerError, err.Error())
	}
	return rpc.OkResponse(req.ID, common.Hash(parsed.ID()).Hex())
}
