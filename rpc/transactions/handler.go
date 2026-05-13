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
	"github.com/vechain/thor/v2/thor"
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
func (h *Handler) Mount(s *rpc.Server) {
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
func (h *Handler) fetchEthTxContext(bestChain *chain.Chain, id thor.Bytes32) (*ethTxContext, error) {
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

func (h *Handler) ethGetTransactionByHash(req rpc.Request) rpc.Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [txHash]")
	}
	id, err := thor.ParseBytes32(params[0])
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid tx hash")
	}

	ctx, err := h.fetchEthTxContext(h.repo.NewBestChain(), id)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	if ctx == nil {
		return rpc.OkResponse(req.ID, nil)
	}

	projIdx := rpc.ProjectedEthIndex(ctx.receipts, ctx.meta.Index)
	return rpc.OkResponse(
		req.ID,
		rpc.ToEthTx(ctx.transaction, h.repo.ChainID(), common.Hash(ctx.header.ID()), uint64(ctx.header.Number()), projIdx, ctx.header.BaseFee()),
	)
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
	ethIdx, err := hexutil.DecodeUint64(idxStr)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid index")
	}

	blk, err := h.repo.GetBlock(header.ID())
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	blockHash := common.Hash(header.ID())
	blockNum := uint64(header.Number())
	var projIdx uint64

	for _, t := range blk.Transactions() {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		if projIdx == ethIdx {
			return rpc.OkResponse(req.ID, rpc.ToEthTx(t, h.repo.ChainID(), blockHash, blockNum, projIdx, header.BaseFee()))
		}
		projIdx++
	}
	return rpc.OkResponse(req.ID, nil)
}

func (h *Handler) ethGetTransactionReceipt(req rpc.Request) rpc.Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [txHash]")
	}
	id, err := thor.ParseBytes32(params[0])
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid tx hash")
	}

	ctx, err := h.fetchEthTxContext(h.repo.NewBestChain(), id)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	if ctx == nil {
		return rpc.OkResponse(req.ID, nil)
	}

	receipt := ctx.receipts[ctx.meta.Index]
	projIdx := rpc.ProjectedEthIndex(ctx.receipts, ctx.meta.Index)
	cumGas := rpc.CumulativeEthGasUsed(ctx.receipts, ctx.meta.Index)
	logOff := rpc.EthLogOffset(ctx.receipts, ctx.meta.Index)

	return rpc.OkResponse(req.ID, rpc.ToEthReceipt(
		ctx.transaction, receipt,
		common.Hash(ctx.header.ID()), uint64(ctx.header.Number()),
		projIdx, cumGas, logOff, ctx.header.BaseFee(),
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

	parsed := new(tx.Transaction)
	if err := parsed.UnmarshalBinary(raw); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}
	// TODO is Adding to the pool enough guarantee for ethereum styled txs ?
	if err := h.txPool.AddLocal(parsed); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeServerError, err.Error())
	}

	return rpc.OkResponse(req.ID, common.Hash(parsed.ID()).Hex())
}
