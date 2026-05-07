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
	"github.com/vechain/thor/v2/rpc"
)

// Handler implements fee market JSON-RPC methods.
type Handler struct {
	repo      *chain.Repository
	backtrace uint32
}

// New creates a fees Handler.
func New(repo *chain.Repository, backtrace uint32) *Handler {
	return &Handler{repo: repo, backtrace: backtrace}
}

// Mount registers all fee market methods on the dispatcher.
func (h *Handler) Mount(d *rpc.Dispatcher) {
	d.Register("eth_gasPrice", h.ethGasPrice)
	d.Register("eth_maxPriorityFeePerGas", h.ethMaxPriorityFeePerGas)
	d.Register("eth_feeHistory", h.ethFeeHistory)
}

func (h *Handler) ethGasPrice(req rpc.Request) rpc.Response {
	header := h.repo.BestBlockSummary().Header
	baseFee := header.BaseFee()
	tip := big.NewInt(1e9) // 1 gwei tip suggestion
	if baseFee == nil {
		return rpc.OkResponse(req.ID, (*hexutil.Big)(tip))
	}
	price := new(big.Int).Add(baseFee, tip)
	return rpc.OkResponse(req.ID, (*hexutil.Big)(price))
}

func (h *Handler) ethMaxPriorityFeePerGas(req rpc.Request) rpc.Response {
	// TODO: derive from on-chain params contract once available.
	return rpc.OkResponse(req.ID, (*hexutil.Big)(big.NewInt(1e9)))
}

func (h *Handler) ethFeeHistory(req rpc.Request) rpc.Response {
	var params []json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 2 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [blockCount, newestBlock, rewardPercentiles]")
	}

	blockCountRaw := params[0]
	var newestRaw string
	if err := json.Unmarshal(params[1], &newestRaw); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid newestBlock")
	}

	// Parse block count (may be hex string or integer)
	var blockCount uint64
	var s string
	if err := json.Unmarshal(blockCountRaw, &s); err == nil {
		n, err := rpc.ParseHexUint64(s)
		if err != nil {
			return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid blockCount")
		}
		blockCount = n
	} else {
		if err := json.Unmarshal(blockCountRaw, &blockCount); err != nil {
			return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid blockCount")
		}
	}

	if blockCount == 0 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "blockCount must be > 0")
	}
	if blockCount > uint64(h.backtrace) {
		blockCount = uint64(h.backtrace)
	}

	newestSummary, err := rpc.ResolveBlockTag(newestRaw, h.repo)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	newestNum := uint64(newestSummary.Header.Number())
	if blockCount > newestNum+1 {
		blockCount = newestNum + 1
	}
	oldestNum := newestNum - blockCount + 1

	bestChain := h.repo.NewBestChain()

	baseFees := make([]*hexutil.Big, 0, blockCount+1)
	gasUsedRatios := make([]float64, 0, blockCount)

	for n := oldestNum; n <= newestNum; n++ {
		hdr, err := bestChain.GetBlockHeader(uint32(n))
		if err != nil {
			return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
		}
		bf := hdr.BaseFee()
		if bf == nil {
			baseFees = append(baseFees, (*hexutil.Big)(new(big.Int)))
		} else {
			baseFees = append(baseFees, (*hexutil.Big)(new(big.Int).Set(bf)))
		}
		ratio := 0.0
		if hdr.GasLimit() > 0 {
			ratio = float64(hdr.GasUsed()) / float64(hdr.GasLimit())
		}
		gasUsedRatios = append(gasUsedRatios, ratio)
	}
	// Include the next block's baseFee (the block after newestBlock).
	// We use the newestBlock's baseFee as an approximation since we don't compute the next one.
	baseFees = append(baseFees, baseFees[len(baseFees)-1])

	type feeHistoryResult struct {
		OldestBlock   hexutil.Uint64 `json:"oldestBlock"`
		BaseFeePerGas []*hexutil.Big `json:"baseFeePerGas"`
		GasUsedRatio  []float64      `json:"gasUsedRatio"`
		Reward        [][]any        `json:"reward"`
	}
	return rpc.OkResponse(req.ID, feeHistoryResult{
		OldestBlock:   hexutil.Uint64(oldestNum),
		BaseFeePerGas: baseFees,
		GasUsedRatio:  gasUsedRatios,
		Reward:        [][]any{},
	})
}
