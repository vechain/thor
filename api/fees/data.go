// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"container/heap"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type Config struct {
	APIBacktraceLimit        uint32
	PriorityBacktraceLimit   uint32
	PrioritySampleTxPerBlock uint32
	PriorityPercentile       uint32
	FixedCacheSize           uint32
}

// minPriorityHeap is a min-heap of priority fee values.
type minPriorityHeap []*big.Int

func (h minPriorityHeap) Len() int           { return len(h) }
func (h minPriorityHeap) Less(i, j int) bool { return h[i].Cmp(h[j]) < 0 }
func (h minPriorityHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *minPriorityHeap) Push(x interface{}) {
	*h = append(*h, x.(*big.Int))
}

func (h *minPriorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type FeeCacheEntry struct {
	parentBlockID thor.Bytes32
	baseFee       *hexutil.Big
	gasUsedRatio  float64
	priorityFees  *minPriorityHeap
}

type FeesData struct {
	repo   *chain.Repository
	cache  *cache.PrioCache
	config Config
}

func newFeesData(repo *chain.Repository, config Config) *FeesData {
	return &FeesData{
		repo:   repo,
		cache:  cache.NewPrioCache(int(config.FixedCacheSize)),
		config: config,
	}
}

func getBaseFee(baseFee *big.Int) *hexutil.Big {
	if baseFee != nil {
		return (*hexutil.Big)(baseFee)
	}
	return (*hexutil.Big)(big.NewInt(0))
}

// resolveRange resolves the base fees, gas used ratios and priority fees for the given block range.
// Assumes that the boundaries (newest block - block count) are correct and validated beforehand.
func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (thor.Bytes32, []*hexutil.Big, []float64, *minPriorityHeap, error) {
	newestBlockID := newestBlockSummary.Header.ID()

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)
	priorityFees := &minPriorityHeap{}
	heap.Init(priorityFees)

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, err := fd.getOrLoadFees(newestBlockID)
		if err != nil {
			return thor.Bytes32{}, nil, nil, nil, err
		}
		baseFees[i-1] = fees.baseFee
		gasUsedRatios[i-1] = fees.gasUsedRatio
		fd.updatePriorityFees(priorityFees, fees.priorityFees, int(fd.config.PrioritySampleTxPerBlock)*int(blockCount))

		newestBlockID = fees.parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, priorityFees, nil
}

func (fd *FeesData) getOrLoadFees(blockID thor.Bytes32) (*FeeCacheEntry, error) {
	fees, _, found := fd.cache.Get(blockID)
	if found {
		return fees.(*FeeCacheEntry), nil
	}

	block, err := fd.repo.GetBlock(blockID)
	if err != nil {
		return nil, err
	}

	header := block.Header()
	transactions := block.Transactions()

	blockPriorityFees := &minPriorityHeap{}
	heap.Init(blockPriorityFees)

	for _, tx := range transactions {
		maxPriorityFeePerGas := fd.effectiveMaxPriorityFeePerGas(tx, header.BaseFee())
		fd.updatePriorityFees(blockPriorityFees, &minPriorityHeap{maxPriorityFeePerGas}, int(fd.config.PrioritySampleTxPerBlock))
	}

	fees = &FeeCacheEntry{
		baseFee:       getBaseFee(header.BaseFee()),
		gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
		parentBlockID: header.ParentID(),
		priorityFees:  blockPriorityFees,
	}
	fd.cache.Set(header.ID(), fees, float64(header.Number()))

	return fees.(*FeeCacheEntry), nil
}

func (fd *FeesData) effectiveMaxPriorityFeePerGas(tx *tx.Transaction, baseFee *big.Int) *big.Int {
	if baseFee == nil {
		return tx.MaxPriorityFeePerGas()
	}
	maxFeePerGas := tx.MaxFeePerGas()
	maxFeePerGas.Sub(maxFeePerGas, baseFee)

	maxPriorityFeePerGas := tx.MaxPriorityFeePerGas()
	if maxPriorityFeePerGas.Cmp(maxFeePerGas) < 0 {
		return maxPriorityFeePerGas
	}
	return maxPriorityFeePerGas
}

func (fd *FeesData) updatePriorityFees(priorityFees, newFees *minPriorityHeap, maxLen int) {
	for _, fee := range *newFees {
		heap.Push(priorityFees, fee)
		if priorityFees.Len() > maxLen {
			heap.Pop(priorityFees)
		}
	}
}
