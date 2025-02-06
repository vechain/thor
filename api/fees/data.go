// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
)

func newFeesData(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedSize uint32) *FeesData {
	return &FeesData{
		repo:           repo,
		cache:          cache.NewPrioCache(int(fixedSize)),
		bft:            bft,
		backtraceLimit: backtraceLimit,
	}
}

func getBaseFee(baseFee *big.Int) *hexutil.Big {
	if baseFee != nil {
		return (*hexutil.Big)(baseFee)
	}
	return (*hexutil.Big)(big.NewInt(0))
}

func (fd *FeesData) pushToCache(header *block.Header) {
	feeCacheEntry := &FeeCacheEntry{
		baseFee:      getBaseFee(header.BaseFee()),
		gasUsedRatio: float64(header.GasUsed()) / float64(header.GasLimit()),
	}
	fd.cache.Set(header.ID(), feeCacheEntry, float64(header.Number()))
}

func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (uint32, []*hexutil.Big, []float64, error) {
	// assumed these are always valid ranges
	newestBlockNumber := newestBlockSummary.Header.Number()
	oldestBlockNumber := newestBlockNumber - blockCount + 1

	// fetch entries from cache
	entries := make([]*cache.PrioEntry, blockCount)
	fd.cache.ForEach(func(ent *cache.PrioEntry) bool {
		if ent.Priority >= float64(oldestBlockNumber) && ent.Priority <= float64(newestBlockNumber) {
			entries[uint32(ent.Priority)-oldestBlockNumber] = ent
		}
		return true
	})

	// return calculated fees and ratios
	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)

	for i, ent := range entries {
		if ent == nil { // value not in cache
			// retrieve from db + retro-populate cache
			blockSummary, err := fd.repo.NewBestChain().GetBlockSummary(oldestBlockNumber + uint32(i))
			if err != nil {
				return 0, nil, nil, err
			}
			fd.pushToCache(blockSummary.Header)

			baseFees[i] = getBaseFee(blockSummary.Header.BaseFee())
			gasUsedRatios[i] = float64(blockSummary.Header.GasUsed()) / float64(blockSummary.Header.GasLimit())
			continue
		}

		// use cached values
		baseFees[i] = getBaseFee((*big.Int)(ent.Value.(*FeeCacheEntry).baseFee))
		gasUsedRatios[i] = ent.Value.(*FeeCacheEntry).gasUsedRatio
	}

	return oldestBlockNumber, baseFees, gasUsedRatios, nil
}
