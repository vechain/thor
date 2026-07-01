// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

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

	if err := validateRewardPercentiles(params.RewardPercentiles); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
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
	var rewards [][]*hexutil.Big
	if len(params.RewardPercentiles) > 0 {
		rewards = make([][]*hexutil.Big, 0, params.BlockCount)
	}

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

		// gasUsedRatio and reward percentiles count only TypeEthDynamicFee gas so
		// Ethereum tooling sees ETH-typed block utilisation and priority fees, not
		// VeChain legacy tx activity.
		receipts, err := h.repo.GetBlockReceipts(hdr.ID())
		if err != nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
		}
		var ethGasUsed uint64
		var rewardItems []rewardItem
		for _, r := range receipts {
			if r.Type != tx.TypeEthDynamicFee {
				continue
			}
			ethGasUsed += r.GasUsed
			if rewards != nil && r.GasUsed > 0 {
				rewardItems = append(rewardItems, rewardItem{
					reward:  new(big.Int).Div(r.Reward, new(big.Int).SetUint64(r.GasUsed)),
					gasUsed: r.GasUsed,
				})
			}
		}
		ratio := 0.0
		if hdr.GasLimit() > 0 {
			ratio = float64(ethGasUsed) / float64(hdr.GasLimit())
		}
		gasUsedRatios = append(gasUsedRatios, ratio)

		if rewards != nil {
			rewards = append(rewards, calcRewardPercentiles(rewardItems, ethGasUsed, params.RewardPercentiles))
		}
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
		Reward:        rewards,
	})
}

// maxRewardPercentiles caps the number of reward percentiles per request.
const maxRewardPercentiles = 100

// rewardItem is a single ETH-typed tx's effective priority fee per gas and its gas used.
type rewardItem struct {
	reward  *big.Int
	gasUsed uint64
}

// validateRewardPercentiles enforces that percentiles are within [0, 100], in
// ascending order, and no more than maxRewardPercentiles entries.
func validateRewardPercentiles(percentiles []float64) error {
	if len(percentiles) > maxRewardPercentiles {
		return fmt.Errorf("there can be at most %d reward percentiles", maxRewardPercentiles)
	}
	for i, p := range percentiles {
		if p < 0 || p > 100 {
			return fmt.Errorf("reward percentile %f is invalid, must be between 0 and 100", p)
		}
		if i > 0 && p < percentiles[i-1] {
			return fmt.Errorf("reward percentiles must be in ascending order, but %f is less than %f", p, percentiles[i-1])
		}
	}
	return nil
}

// calcRewardPercentiles returns, for each requested percentile, the effective
// priority fee per gas of the ETH-typed tx at that cumulative-gas threshold.
// items must already be filtered to ETH-typed txs; totalGasUsed is their summed
// gas. Blocks with no ETH-typed txs yield a zero for every percentile.
func calcRewardPercentiles(items []rewardItem, totalGasUsed uint64, percentiles []float64) []*hexutil.Big {
	rewards := make([]*hexutil.Big, len(percentiles))
	if len(items) == 0 {
		for i := range rewards {
			rewards[i] = (*hexutil.Big)(new(big.Int))
		}
		return rewards
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].reward.Cmp(items[j].reward) < 0
	})

	txIndex := 0
	cumulativeGasUsed := items[0].gasUsed
	for i, p := range percentiles {
		thresholdGasUsed := uint64(float64(totalGasUsed) * p / 100)
		for cumulativeGasUsed < thresholdGasUsed && txIndex < len(items)-1 {
			txIndex++
			cumulativeGasUsed += items[txIndex].gasUsed
		}
		rewards[i] = (*hexutil.Big)(new(big.Int).Set(items[txIndex].reward))
	}
	return rewards
}
