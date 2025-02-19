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
	priorityPercentile          = 60
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

type FeeHistoryCacheEntry struct {
	parentBlockID thor.Bytes32
	baseFee       *hexutil.Big
	gasUsedRatio  float64
}

type FeePriorityCacheEntry struct {
	parentBlockID thor.Bytes32
	priorityFee   *hexutil.Big
}

type FeesData struct {
	repo          *chain.Repository
	historyCache  *cache.PrioCache
	priorityCache *cache.PrioCache
}

func newFeesData(repo *chain.Repository, fixedSize uint32) *FeesData {
	return &FeesData{
		repo:          repo,
		historyCache:  cache.NewPrioCache(int(fixedSize)),
		priorityCache: cache.NewPrioCache(int(priorityNumberOfBlocks) * int(priorityNumberOfTxsPerBlock)),
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

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, _, found := fd.historyCache.Get(newestBlockID)
		if !found {
			// retrieve from db + retro-populate cache
			blockSummary, err := fd.repo.GetBlockSummary(newestBlockID)
			if err != nil {
				return thor.Bytes32{}, nil, nil, err
			}

			header := blockSummary.Header
			fees = &FeeHistoryCacheEntry{
				baseFee:       getBaseFee(header.BaseFee()),
				gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
				parentBlockID: header.ParentID(),
			}
			fd.historyCache.Set(header.ID(), fees, float64(header.Number()))
		}
		baseFees[i-1] = fees.(*FeeHistoryCacheEntry).baseFee
		gasUsedRatios[i-1] = fees.(*FeeHistoryCacheEntry).gasUsedRatio

		newestBlockID = fees.(*FeeHistoryCacheEntry).parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, nil
}

// getPriorityFee returns the priority fee from best block - the priorityNumberOfBlocks constant.
func (fd *FeesData) getPriorityFee() (*hexutil.Big, error) {
	currentBlockHeader := fd.repo.BestBlockSummary().Header
	currentBlockID := currentBlockHeader.ID()
	priorityFeeEntry, _, found := fd.priorityCache.Get(currentBlockID)
	if found {
		return priorityFeeEntry.(*FeePriorityCacheEntry).priorityFee, nil
	}

	priorityFees := &minPriorityHeap{}
	heap.Init(priorityFees)

	for i := priorityNumberOfBlocks; i > 0; i-- {
		// retrieve from db + retro-populate cache
		block, err := fd.repo.GetBlock(currentBlockID)
		if err != nil {
			return nil, err
		}

		header := block.Header()
		transactions := block.Transactions()

		// Use a min-heap to keep track of the lowest values
		blockPriorityFees := &minPriorityHeap{}
		heap.Init(blockPriorityFees)

		for _, tx := range transactions {
			maxPriorityFeePerGas := tx.MaxPriorityFeePerGas()
			// TODO: explore other cases where maxPriorityFeePerGas is smaller than baseFee
			maxPriorityFeePerGas.Sub(maxPriorityFeePerGas, header.BaseFee())
			heap.Push(blockPriorityFees, maxPriorityFeePerGas)
			if blockPriorityFees.Len() > priorityNumberOfTxsPerBlock {
				heap.Pop(blockPriorityFees)
			}
		}

		for _, blockPriorityFee := range blockPriorityFees.GetAllValues() {
			heap.Push(priorityFees, blockPriorityFee)
			if priorityFees.Len() > priorityNumberOfTxsPerBlock*priorityNumberOfBlocks {
				heap.Pop(priorityFees)
			}
		}

		currentBlockID = header.ParentID()
	}

	priorityFeesValues := priorityFees.GetAllValues()
	priorityFeeEntry = priorityFeesValues[(len(priorityFeesValues)-1)*priorityPercentile/100]
	priorityFee := (*hexutil.Big)(priorityFeeEntry.(*big.Int))

	fd.priorityCache.Set(currentBlockID, &FeePriorityCacheEntry{
		parentBlockID: currentBlockHeader.ParentID(),
		priorityFee:   priorityFee,
	}, float64(currentBlockHeader.Number()))

	return priorityFee, nil
}
