// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"container/heap"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
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

// GetAllValues returns all the values in the heap.
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

// resolveRange resolves the base fees, gas used ratios and priority fees for the given block range.
// Assumes that the boundaries (newest block - block count) are correct and validated beforehand.
func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (thor.Bytes32, []*hexutil.Big, []float64, *hexutil.Big, error) {
	newestBlockID := newestBlockSummary.Header.ID()

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)
	priorityFees := &minPriorityHeap{}
	heap.Init(priorityFees)

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, err := fd.pushFeesToCache(newestBlockID)
		if err != nil {
			return thor.Bytes32{}, nil, nil, nil, err
		}
		baseFees[i-1] = fees.baseFee
		gasUsedRatios[i-1] = fees.gasUsedRatio
		for _, blockPriorityFee := range *fees.priorityFees {
			heap.Push(priorityFees, blockPriorityFee)
			if priorityFees.Len() > priorityNumberOfTxsPerBlock*int(blockCount) {
				heap.Pop(priorityFees)
			}
		}

		newestBlockID = fees.parentBlockID
	}

	if priorityFees.Len() > 0 {
		priorityFeeEntry := priorityFees.GetAllValues()[(priorityFees.Len()-1)*priorityPercentile/100]
		priorityFee := (*hexutil.Big)(priorityFeeEntry)
		return oldestBlockID, baseFees, gasUsedRatios, priorityFee, nil
	}

	return oldestBlockID, baseFees, gasUsedRatios, nil, nil
}

func (fd *FeesData) pushFeesToCache(blockID thor.Bytes32) (*FeeCacheEntry, error) {
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
		maxPriorityFeePerGas, _ := fd.EffectiveMaxPriorityFeePerGas(tx, header.BaseFee())
		heap.Push(blockPriorityFees, maxPriorityFeePerGas)
		if blockPriorityFees.Len() > priorityNumberOfTxsPerBlock {
			heap.Pop(blockPriorityFees)
		}
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

func (fd *FeesData) EffectiveMaxPriorityFeePerGas(tx *tx.Transaction, baseFee *big.Int) (*big.Int, error) {
	if baseFee == nil {
		return tx.MaxPriorityFeePerGas(), nil
	}
	var err error
	maxFeePerGas := tx.MaxFeePerGas()
	if maxFeePerGas.Cmp(baseFee) < 0 {
		err = errors.New("maxFeePerGas less than base fee")
	}
	maxFeePerGas.Sub(maxFeePerGas, baseFee)

	maxPriorityFeePerGas := tx.MaxPriorityFeePerGas()
	if maxPriorityFeePerGas.Cmp(maxFeePerGas) < 0 {
		return maxPriorityFeePerGas, err
	}
	return maxPriorityFeePerGas, err
}
