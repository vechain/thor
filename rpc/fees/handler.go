// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Handler implements fee market JSON-RPC methods.
type Handler struct {
	repo       *chain.Repository
	backtrace  uint32
	forkConfig *thor.ForkConfig
}

// New creates a fees Handler.
func New(repo *chain.Repository, backtrace uint32, forkConfig *thor.ForkConfig) *Handler {
	return &Handler{repo: repo, backtrace: backtrace, forkConfig: forkConfig}
}

// Mount registers all fee market methods on the dispatcher.
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_gasPrice", h.ethGasPrice)
	s.Register("eth_maxPriorityFeePerGas", h.ethMaxPriorityFeePerGas)
	s.Register("eth_feeHistory", h.ethFeeHistory)
}

func (h *Handler) ethGasPrice(req jsonrpc.Request) jsonrpc.Response {
	header := h.repo.BestBlockSummary().Header
	baseFee := header.BaseFee()
	tip := big.NewInt(1e9) // 1 gwei tip suggestion
	if baseFee == nil {
		return jsonrpc.OkResponse(req.ID, (*hexutil.Big)(tip))
	}
	price := new(big.Int).Add(baseFee, tip)
	return jsonrpc.OkResponse(req.ID, (*hexutil.Big)(price))
}

func (h *Handler) ethMaxPriorityFeePerGas(req jsonrpc.Request) jsonrpc.Response {
	// TODO: derive from on-chain params contract once available.
	return jsonrpc.OkResponse(req.ID, (*hexutil.Big)(big.NewInt(1e9)))
}

func (h *Handler) ethFeeHistory(req jsonrpc.Request) jsonrpc.Response {
	var params rpc.FeeHistoryParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}

	// Reward percentiles are not yet supported.
	if len(params.RewardPercentiles) > 0 {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, "reward percentiles are not yet supported")
	}

	if params.BlockCount == 0 {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "blockCount must be > 0")
	}
	if params.BlockCount > uint64(h.backtrace) {
		params.BlockCount = uint64(h.backtrace)
	}

	newestSummary, err := ethconvert.ResolveBlockTag(params.NewestBlock, h.repo)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}

	newestNum := uint64(newestSummary.Header.Number())
	if params.BlockCount > newestNum+1 {
		params.BlockCount = newestNum + 1
	}
	oldestNum := newestNum - params.BlockCount + 1

	bestChain := h.repo.NewBestChain()

	baseFees := make([]*hexutil.Big, 0, params.BlockCount+1)
	gasUsedRatios := make([]float64, 0, params.BlockCount)

	for n := oldestNum; n <= newestNum; n++ {
		hdr, err := bestChain.GetBlockHeader(uint32(n))
		if err != nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
		}
		bf := hdr.BaseFee()
		if bf == nil {
			baseFees = append(baseFees, (*hexutil.Big)(new(big.Int)))
		} else {
			baseFees = append(baseFees, (*hexutil.Big)(new(big.Int).Set(bf)))
		}

		// gasUsedRatio counts only TypeEthDynamicFee gas so Ethereum tooling sees
		// ETH-typed block utilisation, not VeChain legacy tx activity.
		receipts, err := h.repo.GetBlockReceipts(hdr.ID())
		if err != nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
		}
		var ethGasUsed uint64
		for _, r := range receipts {
			if r.Type == tx.TypeEthDynamicFee {
				ethGasUsed += r.GasUsed
			}
		}
		ratio := 0.0
		if hdr.GasLimit() > 0 {
			ratio = float64(ethGasUsed) / float64(hdr.GasLimit())
		}
		gasUsedRatios = append(gasUsedRatios, ratio)
	}

	// Compute the true next-block baseFee using the consensus formula.
	nextBaseFee := galactica.CalcBaseFee(newestSummary.Header, h.forkConfig)
	if nextBaseFee == nil {
		nextBaseFee = new(big.Int)
	}
	baseFees = append(baseFees, (*hexutil.Big)(nextBaseFee))

	return jsonrpc.OkResponse(req.ID, rpc.FeeHistoryResult{
		OldestBlock:   hexutil.Uint64(oldestNum),
		BaseFeePerGas: baseFees,
		GasUsedRatio:  gasUsedRatios,
	})
}
