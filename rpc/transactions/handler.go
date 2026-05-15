// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// Handler implements transaction JSON-RPC methods.
type Handler struct {
	repo   *chain.Repository
	txPool txpool.Pool
}

// New creates a transactions Handler.
func New(repo *chain.Repository, txPool txpool.Pool) *Handler {
	return &Handler{repo: repo, txPool: txPool}
}

// Mount registers all transaction methods on the dispatcher.
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_getTransactionByHash", h.ethGetTransactionByHash)
	s.Register("eth_getTransactionByBlockHashAndIndex", h.ethGetTransactionByBlockHashAndIndex)
	s.Register("eth_getTransactionByBlockNumberAndIndex", h.ethGetTransactionByBlockNumberAndIndex)
	s.Register("eth_getTransactionReceipt", h.ethGetTransactionReceipt)
	s.Register("eth_sendRawTransaction", h.ethSendRawTransaction)
}

type ethTxContext struct {
	transaction *tx.Transaction
	meta        *chain.TxMeta
	header      *block.Header
	receipts    tx.Receipts
}

// fetchEthTxContext looks up an ETH-typed tx by hash and loads its block header and receipts.
// Returns nil, nil when the tx does not exist or is not an ETH-typed transaction.
func (h *Handler) fetchEthTxContext(bestChain *chain.Chain, id [32]byte) (*ethTxContext, error) {
	t, meta, err := bestChain.GetTransaction(id)
	if err != nil || t.Type() != tx.TypeEthDynamicFee {
		return nil, nil
	}
	header, err := bestChain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return nil, err
	}
	receipts, err := h.repo.GetBlockReceipts(header.ID())
	if err != nil {
		return nil, err
	}
	return &ethTxContext{transaction: t, meta: meta, header: header, receipts: receipts}, nil
}

func (h *Handler) ethGetTransactionByHash(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.TxHashParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	ctx, err := h.fetchEthTxContext(h.repo.NewBestChain(), params.Hash)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	if ctx == nil {
		return jsonrpc.OkResponse(req.ID, nil)
	}

	projIdx := ethconvert.ProjectedEthIndex(ctx.receipts, ctx.meta.Index)
	return jsonrpc.OkResponse(req.ID, ethconvert.ToEthTx(
		ctx.transaction, h.repo.ChainID(),
		common.Hash(ctx.header.ID()), uint64(ctx.header.Number()),
		projIdx, ctx.header.BaseFee(),
	))
}

func (h *Handler) ethGetTransactionByBlockHashAndIndex(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagAndIndexParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	summary, err := ethconvert.ResolveBlockTag(params.Tag, h.repo)
	if err != nil {
		return jsonrpc.OkResponse(req.ID, nil)
	}
	return h.txByBlockAndEthIndex(req, summary.Header, params.Index)
}

func (h *Handler) ethGetTransactionByBlockNumberAndIndex(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.BlockTagAndIndexParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	summary, err := ethconvert.ResolveBlockTag(params.Tag, h.repo)
	if err != nil {
		return jsonrpc.OkResponse(req.ID, nil)
	}
	return h.txByBlockAndEthIndex(req, summary.Header, params.Index)
}

func (h *Handler) txByBlockAndEthIndex(req jsonrpc.Request, header *block.Header, ethIdx uint64) jsonrpc.Response {
	blk, err := h.repo.GetBlock(header.ID())
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}

	blockHash := common.Hash(header.ID())
	blockNum := uint64(header.Number())
	var projIdx uint64

	for _, t := range blk.Transactions() {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		if projIdx == ethIdx {
			return jsonrpc.OkResponse(req.ID, ethconvert.ToEthTx(t, h.repo.ChainID(), blockHash, blockNum, projIdx, header.BaseFee()))
		}
		projIdx++
	}
	return jsonrpc.OkResponse(req.ID, nil)
}

func (h *Handler) ethGetTransactionReceipt(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.TxHashParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	ctx, err := h.fetchEthTxContext(h.repo.NewBestChain(), params.Hash)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	if ctx == nil {
		return jsonrpc.OkResponse(req.ID, nil)
	}

	receipt := ctx.receipts[ctx.meta.Index]
	projIdx := ethconvert.ProjectedEthIndex(ctx.receipts, ctx.meta.Index)
	cumGas := ethconvert.CumulativeEthGasUsed(ctx.receipts, ctx.meta.Index)
	logOff := ethconvert.EthLogOffset(ctx.receipts, ctx.meta.Index)

	return jsonrpc.OkResponse(req.ID, ethconvert.ToEthReceipt(
		ctx.transaction, receipt,
		common.Hash(ctx.header.ID()), uint64(ctx.header.Number()),
		projIdx, cumGas, logOff, ctx.header.BaseFee(),
	))
}

func (h *Handler) ethSendRawTransaction(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.RawTxParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	parsed := new(tx.Transaction)
	if err := parsed.UnmarshalBinary(params.Raw); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	if parsed.Type() != tx.TypeEthDynamicFee {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "only EIP-1559 (type 2) transactions are accepted")
	}
	if err := h.txPool.AddLocal(parsed); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, err.Error())
	}
	return jsonrpc.OkResponse(req.ID, common.Hash(parsed.ID()).Hex())
}
