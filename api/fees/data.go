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
	parentBlockID thor.Bytes32
	baseFee       *hexutil.Big
	gasUsedRatio  float64
}

type FeesData struct {
	repo  *chain.Repository
	cache *cache.PrioCache
}

func newFeesData(repo *chain.Repository, fixedCacheSize int) *FeesData {
	return &FeesData{
		repo:  repo,
		cache: cache.NewPrioCache(fixedCacheSize),
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
func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (thor.Bytes32, []*hexutil.Big, []float64, error) {
	newestBlockID := newestBlockSummary.Header.ID()

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, err := fd.getOrLoadFees(newestBlockID)
		if err != nil {
			// This should happen only when "next" since the boundaries are validated beforehand
			// We do not cache the data in this case since it does not belong to an actual block
			if fd.repo.IsNotFound(err) {
				header := newestBlockSummary.Header
				baseFees[i-1] = getBaseFee(header.BaseFee())
				gasUsedRatios[i-1] = float64(header.GasUsed()) / float64(header.GasLimit())
				newestBlockID = header.ParentID()
				continue
			}
			return thor.Bytes32{}, nil, nil, err
		}
		baseFees[i-1] = fees.baseFee
		gasUsedRatios[i-1] = fees.gasUsedRatio

		newestBlockID = fees.parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, nil
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

	fees = &FeeCacheEntry{
		baseFee:       getBaseFee(header.BaseFee()),
		gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
		parentBlockID: header.ParentID(),
	}
	fd.cache.Set(header.ID(), fees, float64(header.Number()))

	return fees.(*FeeCacheEntry), nil
}
