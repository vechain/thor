// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type FeeCacheEntry struct {
	baseFee       *hexutil.Big
	gasUsedRatio  float64
	parentBlockID thor.Bytes32
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

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, _, found := fd.cache.Get(newestBlockID)
		if !found {
			// retrieve from db + retro-populate cache
			blockSummary, err := fd.repo.GetBlockSummary(newestBlockID)
			if err != nil {
				return thor.Bytes32{}, nil, nil, err
			}

			header := blockSummary.Header
			fees = &FeeCacheEntry{
				baseFee:       getBaseFee(header.BaseFee()),
				gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
				parentBlockID: header.ParentID(),
			}
			fd.cache.Set(header.ID(), fees, float64(header.Number()))
		}
		baseFees[i-1] = fees.(*FeeCacheEntry).baseFee
		gasUsedRatios[i-1] = fees.(*FeeCacheEntry).gasUsedRatio

		newestBlockID = fees.(*FeeCacheEntry).parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, nil
}
