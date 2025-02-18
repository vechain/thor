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
)

const (
	priorityNumberOfBlocks      = 20
	priorityNumberOfTxsPerBlock = 3
	priorityPercentile          = 0.6
)

// minPriorityHeap is a min-heap of *big.Int values.
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

func (h minPriorityHeap) GetAllValues() []*big.Int {
    values := make([]*big.Int, len(h))
    copy(values, h)
    return values
}

type FeeCacheEntry struct {
	parentBlockID thor.Bytes32
	baseFee       *hexutil.Big
	gasUsedRatio  float64
	priorityFees  *minPriorityHeap
}
type FeesData struct {
	repo  *chain.Repository
	cache *cache.PrioCache
}

func newFeesData(repo *chain.Repository, fixedSize uint32) *FeesData {
	return &FeesData{
		repo:  repo,
		cache: cache.NewPrioCache(int(fixedSize)),
	}
}

func getBaseFee(baseFee *big.Int) *hexutil.Big {
	if baseFee != nil {
		return (*hexutil.Big)(baseFee)
	}
	return (*hexutil.Big)(big.NewInt(0))
}

// resolveRange resolves the base fees and gas used ratios for the given block range.
// Assumes that the boundaries (newest block - block count) are correct and validated beforehand.
func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (thor.Bytes32, []*hexutil.Big, []float64, error) {
	newestBlockID := newestBlockSummary.Header.ID()

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)
	priorityFees := make([][]*big.Int, priorityNumberOfTxsPerBlock)

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, _, found := fd.cache.Get(newestBlockID)
		if !found {
			// retrieve from db + retro-populate cache
			block, err := fd.repo.GetBlock(newestBlockID)
			if err != nil {
				return thor.Bytes32{}, nil, nil, err
			}

			header := block.Header()
			transactions := block.Transactions()

			// Use a min-heap to keep track of the lowest values
			priorityFees := &minPriorityHeap{}
			heap.Init(priorityFees)

			for _, tx := range transactions {
				fee := tx.MaxPriorityFeePerGas()
				// TODO: explore other cases where maxPriorityFeePerGas is smaller than baseFee
				fee.Sub(fee, header.BaseFee())
				heap.Push(priorityFees, fee)
				if priorityFees.Len() > priorityNumberOfTxsPerBlock {
					heap.Pop(priorityFees)
				}
			}
			fees = &FeeCacheEntry{
				baseFee:       getBaseFee(header.BaseFee()),
				gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
				parentBlockID: header.ParentID(),
				priorityFees:  priorityFees,
			}
			fd.cache.Set(header.ID(), fees, float64(header.Number()))
		}
		baseFees[i-1] = fees.(*FeeCacheEntry).baseFee
		gasUsedRatios[i-1] = fees.(*FeeCacheEntry).gasUsedRatio
		priorityFees[i-1] = fees.(*FeeCacheEntry).priorityFees.GetAllValues()

		newestBlockID = fees.(*FeeCacheEntry).parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, nil
}
