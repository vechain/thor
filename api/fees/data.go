// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
)

func newFeesData(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedSize uint32) *FeesData {
	cacheSize := int(math.Min(float64(backtraceLimit), float64(fixedSize)))
	maxBacktraceLimit := uint32(math.Max(float64(backtraceLimit), float64(cacheSize)))
	return &FeesData{
		repo:              repo,
		cache:             cache.NewPrioCache(cacheSize),
		bft:               bft,
		cacheSize:         uint32(cacheSize),
		maxBacktraceLimit: maxBacktraceLimit,
	}
}

func newFeesCacheEntry(header *block.Header) *FeeCacheEntry {
	baseFee := header.BaseFee()
	if baseFee == nil {
		baseFee = big.NewInt(0)
	}
	return &FeeCacheEntry{
		baseFee:      (*hexutil.Big)(baseFee),
		gasUsedRatio: float64(header.GasUsed()) / float64(header.GasLimit()),
	}
}

func (fd *FeesData) pushToCache(header *block.Header) {
	fd.cache.Set(header.ID(), newFeesCacheEntry(header), float64(header.Number()))
}

func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (uint32, []*hexutil.Big, []float64, error) {
	newestBlockSummaryNumber := newestBlockSummary.Header.Number()
	entries := make([]*cache.PrioEntry, blockCount)
	oldestBlockNumber := uint32(math.Max(0, float64(int(newestBlockSummaryNumber)+1-int(blockCount))))
	fd.cache.ForEach(func(ent *cache.PrioEntry) bool {
		if ent.Priority >= float64(oldestBlockNumber) && ent.Priority <= float64(newestBlockSummaryNumber) {
			entries[uint32(ent.Priority)-oldestBlockNumber] = ent
		}
		return true
	})

	backtraceBlockNumber := uint32(math.Max(0, float64(int(fd.repo.BestBlockSummary().Header.Number())-int(fd.maxBacktraceLimit)+1)))
	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)
	for i, ent := range entries {
		if oldestBlockNumber+uint32(i) < backtraceBlockNumber {
			continue
		}
		if ent == nil {
			blockRevision, err := utils.ParseRevision(strconv.FormatUint(uint64(i+int(oldestBlockNumber)), 10), false)
			if err != nil {
				return 0, nil, nil, err
			}
			blockSummary, err := utils.GetSummary(blockRevision, fd.repo, fd.bft)
			if err != nil {
				if !fd.repo.IsNotFound(err) {
					return 0, nil, nil, err
				}
			} else {
				fd.pushToCache(blockSummary.Header)

				if baseFee := blockSummary.Header.BaseFee(); baseFee != nil {
					baseFees[i] = (*hexutil.Big)(baseFee)
				} else {
					baseFees[i] = (*hexutil.Big)(big.NewInt(0))
				}

				gasUsedRatios[i] = float64(blockSummary.Header.GasUsed()) / float64(blockSummary.Header.GasLimit())
			}
		} else {
			if baseFee := ent.Value.(*FeeCacheEntry).baseFee; baseFee != nil {
				baseFees[i] = baseFee
			} else {
				baseFees[i] = (*hexutil.Big)(big.NewInt(0))
			}
			gasUsedRatios[i] = ent.Value.(*FeeCacheEntry).gasUsedRatio
		}
	}

	numberOfEntriesToOmit := uint32(math.Max(0, float64(int(backtraceBlockNumber)-int(oldestBlockNumber))))
	numElements := newestBlockSummaryNumber - oldestBlockNumber + 1

	return oldestBlockNumber + numberOfEntriesToOmit, baseFees[numberOfEntriesToOmit:numElements], gasUsedRatios[numberOfEntriesToOmit:numElements], nil
}
