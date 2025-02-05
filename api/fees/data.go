// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

func newFeesData(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedSize uint32) *FeesData {
	size := int(math.Min(float64(backtraceLimit), float64(fixedSize)))
	return &FeesData{
		repo:           repo,
		cache:          cache.NewPrioCache(size),
		bft:            bft,
		size:           size,
		backtraceLimit: backtraceLimit,
	}
}

func newFeesCacheEntry(header *block.Header) *FeeCacheEntry {
	return &FeeCacheEntry{
		baseFee:      (*hexutil.Big)(header.BaseFee()),
		gasUsedRatio: float64(header.GasUsed()) / float64(header.GasLimit()),
	}
}

func (fd *FeesData) pushToCache(header *block.Header) {
	fd.cache.Set(header.ID(), newFeesCacheEntry(header), float64(header.Number()))
}

func (fd *FeesData) resolveRange(oldestBlockSummary *chain.BlockSummary, newestBlockSummary *chain.BlockSummary, blockCount uint32) (uint32, []*hexutil.Big, []float64, error) {
	cacheOldestBlockNumber, cacheBaseFees, cacheGasUsedRatios, shouldGetBlockSummaries, err := fd.resolveRangeCache(oldestBlockSummary.Header.ID(), newestBlockSummary.Header.Number())
	if !shouldGetBlockSummaries && err != nil {
		return 0, nil, nil, err
	}

	if shouldGetBlockSummaries {
		// Get block summaries for the missing blocks
		newestBlockSummaryNumber := cacheOldestBlockNumber - 1
		summariesGasFees, summariesGasUsedRatios, err := fd.getBlockSummaries(newestBlockSummaryNumber, blockCount-uint32(len(cacheBaseFees)))
		if err != nil {
			return 0, nil, nil, err
		}
		oldestBlockSummary := oldestBlockSummary.Header.Number()
		return oldestBlockSummary, append(summariesGasFees, cacheBaseFees...), append(summariesGasUsedRatios, cacheGasUsedRatios...), nil
	}

	return cacheOldestBlockNumber, cacheBaseFees, cacheGasUsedRatios, nil
}

func (fd *FeesData) resolveRangeCache(oldestBlockID thor.Bytes32, lastBlockNumber uint32) (uint32, []*hexutil.Big, []float64, bool, error) {
	// The newest block will also be in the cache, so we do not check it at this stage
	shouldGetBlockSummaries := false

	// First case: the oldest block is in the cache
	_, oldestBlockNumberFloat64, oldestBlockInCache := fd.cache.Get(oldestBlockID)
	var oldestBlockNumber uint32
	entries := make([]*cache.PrioEntry, 0, fd.cache.Len())
	if oldestBlockInCache {
		fd.cache.ForEach(func(ent *cache.PrioEntry) bool {
			if ent.Priority >= oldestBlockNumberFloat64 && ent.Priority <= float64(lastBlockNumber) {
				entries = append(entries, ent)
			}
			return true
		})
		oldestBlockNumber = uint32(oldestBlockNumberFloat64)
	} else {
		// Second case: the oldest block is not in the cache
		// Meaning that we should get everything from the last block downwards
		fd.cache.ForEach(func(ent *cache.PrioEntry) bool {
			if ent.Priority <= float64(lastBlockNumber) {
				entries = append(entries, ent)
			}
			return true
		})

		// Only check block summaries if the backtrace limit is higher
		// We should get the block summaries from the oldest block to oldest block - 1 in the cache
		shouldGetBlockSummaries = fd.cache.Len() < fd.size || fd.size < int(fd.backtraceLimit)
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

	return 1, nil, nil, shouldGetBlockSummaries, errors.New("no block fees found in cache for the given range")
}

func (fd *FeesData) processBlockSummaries(next *atomic.Uint32, lastBlock uint32, blockDataChan chan *blockData) {
	for {
		// Processing current block and incrementing next block number at the same time
		blockNumber := next.Add(1) - 1
		if blockNumber > lastBlock {
			return
		}
		blockFee := &blockData{}
		blockFee.blockRevision, blockFee.err = utils.ParseRevision(strconv.FormatUint(uint64(blockNumber), 10), false)
		if blockFee.err == nil {
			blockFee.blockSummary, blockFee.err = utils.GetSummary(blockFee.blockRevision, fd.repo, fd.bft)
			if blockFee.blockSummary == nil {
				blockFee.err = fmt.Errorf("block summary is nil for block number %d", blockNumber)
			}
		}
		blockDataChan <- blockFee
	}
}

func (fd *FeesData) processBlockRange(blockCount uint32, summary *chain.BlockSummary) (uint32, chan *blockData) {
	lastBlock := summary.Header.Number()
	oldestBlockInt32 := int32(lastBlock) + 1 - int32(blockCount)
	oldestBlock := uint32(0)
	if oldestBlockInt32 >= 0 {
		oldestBlock = uint32(oldestBlockInt32)
	}
	var next atomic.Uint32
	next.Store(oldestBlock)

	blockDataChan := make(chan *blockData, blockCount)
	var wg sync.WaitGroup

	for i := 0; i < maxBlockFetchers && i < int(blockCount); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fd.processBlockSummaries(&next, lastBlock, blockDataChan)
		}()
	}

	go func() {
		wg.Wait()
		close(blockDataChan)
	}()

	return oldestBlock, blockDataChan
}

func (fd *FeesData) getBlockSummaries(newestBlockSummaryNumber uint32, blockCount uint32) ([]*hexutil.Big, []float64, error) {
	newestBlockRevision := utils.NewRevision(newestBlockSummaryNumber)
	summary, err := utils.GetSummary(newestBlockRevision, fd.repo, fd.bft)
	if err != nil {
		return nil, nil, err
	}

	oldestBlock, blockDataChan := fd.processBlockRange(blockCount, summary)

	var (
		baseFeesWithNil = make([]*hexutil.Big, blockCount)
		gasUsedRatios   = make([]float64, blockCount)
	)

	// Collect results from the channel
	for blockData := range blockDataChan {
		if blockData.err != nil {
			return nil, nil, blockData.err
		}
		// Since it is a priority cache, we will store only the top blocks
		// So the cache is rebuilt when calling the endpoint if the node went down
		fd.pushToCache(blockData.blockSummary.Header)
		// Ensure the order of the baseFees and gasUsedRatios is correct
		blockPosition := blockData.blockSummary.Header.Number() - oldestBlock
		if baseFee := blockData.blockSummary.Header.BaseFee(); baseFee != nil {
			baseFeesWithNil[blockPosition] = (*hexutil.Big)(baseFee)
		} else {
			baseFeesWithNil[blockPosition] = (*hexutil.Big)(big.NewInt(0))
		}
		gasUsedRatios[blockPosition] = float64(blockData.blockSummary.Header.GasUsed()) / float64(blockData.blockSummary.Header.GasLimit())
	}

	// Remove nil values from baseFees
	var baseFees []*hexutil.Big
	for _, baseFee := range baseFeesWithNil {
		if baseFee != nil {
			baseFees = append(baseFees, baseFee)
		}
	}

	return baseFees, gasUsedRatios[:len(baseFees)], nil
}
