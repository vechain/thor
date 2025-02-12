// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/utils"
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
		baseFee:       getBaseFee(header.BaseFee()),
		gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
		parentBlockID: header.ParentID(),
	}
	fd.cache.Set(header.ID(), feeCacheEntry, float64(header.Number()))
}

func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32) (*utils.Revision, []*hexutil.Big, []float64, error) {
	newestBlockID, err := fd.repo.NewBestChain().GetBlockID(newestBlockSummary.Header.Number())
	if err != nil {
		return nil, nil, nil, err
	}

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)

	for i := blockCount; i > 0; i-- {
		fees, _, found := fd.cache.Get(newestBlockID)
		if !found {
			// retrieve from db + retro-populate cache
			blockSummary, err := fd.repo.GetBlockSummary(newestBlockID)
			if err != nil {
				return nil, nil, nil, err
			}

			fd.pushToCache(blockSummary.Header)

			baseFees[i-1] = getBaseFee(blockSummary.Header.BaseFee())
			gasUsedRatios[i-1] = float64(blockSummary.Header.GasUsed()) / float64(blockSummary.Header.GasLimit())

			newestBlockID = blockSummary.Header.ParentID()
		} else {
			baseFees[i-1] = getBaseFee((*big.Int)(fees.(*FeeCacheEntry).baseFee))
			gasUsedRatios[i-1] = fees.(*FeeCacheEntry).gasUsedRatio

			newestBlockID = fees.(*FeeCacheEntry).parentBlockID
		}
	}

	oldestBlockID, err := fd.repo.NewBestChain().GetBlockID(newestBlockSummary.Header.Number() - blockCount + 1)
	if err != nil {
		return nil, nil, nil, err
	}

	return utils.NewRevision(oldestBlockID), baseFees, gasUsedRatios, nil
}
