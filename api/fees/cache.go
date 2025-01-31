// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"errors"
	"math"
	"sort"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

func NewFeesCache(repo *chain.Repository, backtraceLimit uint32, fixedSize uint32) *FeesCache {
	size := int(math.Min(float64(backtraceLimit), float64(fixedSize)))
	return &FeesCache{
		repo:           repo,
		cache:          cache.NewPrioCache(size),
		size:           size,
		backtraceLimit: backtraceLimit,
		fixedSize:      fixedSize,
	}
}

func newFeesCacheEntry(header *block.Header) *FeeCacheEntry {
	return &FeeCacheEntry{
		baseFee:      (*hexutil.Big)(header.BaseFee()),
		gasUsedRatio: float64(header.GasUsed()) / float64(header.GasLimit()),
	}
}

func (fc *FeesCache) get(blockID thor.Bytes32) (*FeeCacheEntry, uint32, error) {
	if feeCacheEntry, blockNumberFloat64, ok := fc.cache.Get(blockID); ok {
		return feeCacheEntry.(*FeeCacheEntry), uint32(blockNumberFloat64), nil
	}

	return nil, 0, errors.New("blocks fees not found")
}

func (fc *FeesCache) getByRevision(rev *utils.Revision, bft bft.Committer) (*FeeCacheEntry, uint32, error) {
	id, err := utils.ParseBlockID(rev, fc.repo, bft)
	if err != nil {
		return nil, 0, err
	}
	return fc.get(id)
}

func (fc *FeesCache) Push(header *block.Header) {
	fc.cache.Set(header.ID(), newFeesCacheEntry(header), float64(header.Number()))
}

func (fc *FeesCache) resolveRange(oldestBlockID thor.Bytes32, lastBlockNumber uint32) (uint32, []*hexutil.Big, []float64, bool, error) {
	// The newest block will also be in the cache, so we do not check it at this stage
	shouldGetBlockSummaries := false

	// First case: the oldest block is in the cache
	_, oldestBlockNumberFloat64, oldestBlockInCache := fc.cache.Get(oldestBlockID)
	var oldestBlockNumber uint32
	entries := make([]*cache.PrioEntry, 0, fc.cache.Len())
	if oldestBlockInCache {
		fc.cache.ForEach(func(ent *cache.PrioEntry) bool {
			if ent.Priority >= oldestBlockNumberFloat64 && ent.Priority <= float64(lastBlockNumber) {
				entries = append(entries, ent)
			}
			return true
		})
		oldestBlockNumber = uint32(oldestBlockNumberFloat64)
	} else {
		// Second case: the oldest block is not in the cache
		// Meaning that we should get everything from the last block downwards
		fc.cache.ForEach(func(ent *cache.PrioEntry) bool {
			if ent.Priority <= float64(lastBlockNumber) {
				entries = append(entries, ent)
			}
			return true
		})

		// Only check block summaries if the backtrace limit is higher
		// We should get the block summaries from the oldest block to oldest block - 1 in the cache
		shouldGetBlockSummaries = fc.size < int(fc.backtraceLimit)
	}

	if len(entries) > 0 {
		// Even though  we have the last blocks in the cache, we need to sort them
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Priority < entries[j].Priority
		})
		baseFees := make([]*hexutil.Big, 0, len(entries))
		gasUsedRatios := make([]float64, 0, len(entries))
		for i, ent := range entries {
			if i == 0 {
				oldestBlockNumber = uint32(ent.Priority)
			}
			baseFees = append(baseFees, ent.Value.(*FeeCacheEntry).baseFee)
			gasUsedRatios = append(gasUsedRatios, ent.Value.(*FeeCacheEntry).gasUsedRatio)
		}
		return oldestBlockNumber, baseFees, gasUsedRatios, shouldGetBlockSummaries, nil
	}

	return 0, nil, nil, shouldGetBlockSummaries, errors.New("block fees could not be resolved for the given range")
}
