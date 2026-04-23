// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/thor"
)

func init() {
	register("eth_gasPrice", handleGasPrice)
	register("eth_maxPriorityFeePerGas", handleMaxPriorityFeePerGas)
	register("eth_feeHistory", handleFeeHistory)
}

const defaultPriorityFeeBps = 5 // 5% of base fee

// priorityFee computes the recommended priority fee per gas. Mirrors the
// existing api/fees logic: if the best header carries a BaseFee and the
// next base fee would exceed InitialBaseFee, use PriorityIncreasePercentage %
// of that forward-projected base fee; otherwise fall back to
// PriorityIncreasePercentage % of InitialBaseFee.
func priorityFee(s *Server) *big.Int {
	pct := int64(s.cfg.PriorityIncreasePercentage)
	if pct == 0 {
		pct = defaultPriorityFeeBps
	}
	min := new(big.Int).Div(new(big.Int).Mul(big.NewInt(thor.InitialBaseFee), big.NewInt(pct)), big.NewInt(100))
	best := s.repo.BestBlockSummary().Header
	if best.BaseFee() == nil {
		return min
	}
	next := galactica.CalcBaseFee(best, s.forkConfig)
	if next.Cmp(big.NewInt(thor.InitialBaseFee)) > 0 {
		return new(big.Int).Div(new(big.Int).Mul(next, big.NewInt(pct)), big.NewInt(100))
	}
	return min
}

// handleMaxPriorityFeePerGas returns the recommended tip per gas as a
// QUANTITY.
func handleMaxPriorityFeePerGas(_ context.Context, s *Server, _ json.RawMessage) (any, *RPCError) {
	return (*hexutil.Big)(priorityFee(s)), nil
}

// handleGasPrice returns base(pending) + priority fee. Approximates Thor's
// REST /fees/priority + /blocks/best baseFee combination.
func handleGasPrice(_ context.Context, s *Server, _ json.RawMessage) (any, *RPCError) {
	best := s.repo.BestBlockSummary().Header
	var base *big.Int
	if best.BaseFee() != nil {
		base = galactica.CalcBaseFee(best, s.forkConfig)
	} else {
		base = big.NewInt(thor.InitialBaseFee)
	}
	total := new(big.Int).Add(base, priorityFee(s))
	return (*hexutil.Big)(total), nil
}

// --- eth_feeHistory ------------------------------------------------------

// feeHistoryResult mirrors Ethereum's JSON-RPC response shape.
type feeHistoryResult struct {
	OldestBlock   hexutil.Uint64   `json:"oldestBlock"`
	BaseFeePerGas []*hexutil.Big   `json:"baseFeePerGas"`
	GasUsedRatio  []float64        `json:"gasUsedRatio"`
	Reward        [][]*hexutil.Big `json:"reward,omitempty"`
}

// handleFeeHistory walks backwards from newestBlock (either a BlockTag, hex
// quantity or tag string) for blockCount blocks. rewardPercentiles is
// accepted and validated but currently emits nil/empty percentile rewards
// (real per-percentile rewards require scanning each block's txs; deferred
// to the Thor-native /fees/history reuse path, Spec 2 Deferred note).
func handleFeeHistory(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var raw []json.RawMessage
	if err := json.Unmarshal(params, &raw); err != nil || len(raw) < 2 || len(raw) > 3 {
		return nil, InvalidParams("expected [blockCount, newestBlock] or [blockCount, newestBlock, rewardPercentiles]")
	}

	// blockCount — hex quantity.
	var bcStr string
	if err := json.Unmarshal(raw[0], &bcStr); err != nil {
		return nil, InvalidParams("blockCount: " + err.Error())
	}
	blockCount, err := parseHexUint64(bcStr)
	if err != nil {
		return nil, InvalidParams("blockCount: " + err.Error())
	}
	if blockCount == 0 {
		return nil, InvalidParams("blockCount must be > 0")
	}
	// Cap at 1024 blocks (Ethereum's de-facto limit).
	if blockCount > 1024 {
		blockCount = 1024
	}

	var tag BlockTag
	if err := json.Unmarshal(raw[1], &tag); err != nil {
		return nil, InvalidParams("newestBlock: " + err.Error())
	}
	_, newestSummary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return nil, ToRPCError(err)
	}
	newestNum := newestSummary.Header.Number()

	// Optional reward percentiles — validated but not used below.
	var percentiles []float64
	if len(raw) == 3 && string(raw[2]) != "null" {
		if err := json.Unmarshal(raw[2], &percentiles); err != nil {
			return nil, InvalidParams("rewardPercentiles: " + err.Error())
		}
		for i, p := range percentiles {
			if p < 0 || p > 100 {
				return nil, InvalidParams("rewardPercentiles values must be in [0,100]")
			}
			if i > 0 && p < percentiles[i-1] {
				return nil, InvalidParams("rewardPercentiles must be ascending")
			}
		}
	}

	// Walk backwards; enforce APIBacktraceLimit.
	if cap := s.cfg.APIBacktraceLimit; cap > 0 && int(blockCount) > cap {
		return nil, ReasonError(ReasonLogRangeTooLarge, fmt.Sprintf("blockCount %d exceeds APIBacktraceLimit %d", blockCount, cap))
	}
	if uint64(newestNum)+1 < blockCount {
		blockCount = uint64(newestNum) + 1
	}
	oldest := uint32(uint64(newestNum) - blockCount + 1)

	baseFees := make([]*hexutil.Big, 0, blockCount+1)
	ratios := make([]float64, 0, blockCount)

	bestChain := s.repo.NewBestChain()
	for n := oldest; n <= newestNum; n++ {
		sum, err := bestChain.GetBlockSummary(n)
		if err != nil {
			return nil, InternalError(err)
		}
		bf := sum.Header.BaseFee()
		if bf == nil {
			bf = new(big.Int)
		}
		baseFees = append(baseFees, (*hexutil.Big)(bf))
		limit := sum.Header.GasLimit()
		if limit == 0 {
			ratios = append(ratios, 0)
		} else {
			ratios = append(ratios, float64(sum.Header.GasUsed())/float64(limit))
		}
	}

	// Forward-project the next base fee so baseFeePerGas has blockCount+1
	// entries (Ethereum's convention). The last entry is the predicted base
	// fee for the block immediately after newestBlock.
	nextBF := galactica.CalcBaseFee(newestSummary.Header, s.forkConfig)
	baseFees = append(baseFees, (*hexutil.Big)(nextBF))

	// Rewards: emit nil-per-block for each requested percentile (spec §13 /
	// §9 — real reward computation requires per-tx tip scanning; reuse of
	// api/fees/data.go is Deferred).
	var rewards [][]*hexutil.Big
	if len(percentiles) > 0 {
		rewards = make([][]*hexutil.Big, 0, blockCount)
		zero := (*hexutil.Big)(new(big.Int))
		for i := uint64(0); i < blockCount; i++ {
			row := make([]*hexutil.Big, len(percentiles))
			for j := range row {
				row[j] = zero
			}
			rewards = append(rewards, row)
		}
	}

	return &feeHistoryResult{
		OldestBlock:   hexutil.Uint64(oldest),
		BaseFeePerGas: baseFees,
		GasUsedRatio:  ratios,
		Reward:        rewards,
	}, nil
}
