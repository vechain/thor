// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"

	"slices"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/thor"
)

type FeeCacheEntry struct {
	parentBlockID thor.Bytes32
	baseFee       *hexutil.Big
	rewards       []*hexutil.Big
	gasUsedRatio  float64
}

type FeesData struct {
	repo  *chain.Repository
	cache *cache.PrioCache
}

type rewardItem struct {
	reward  *big.Int
	gasUsed uint64
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
func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32, rewardPercentiles *[]float64, baseGasPrice *big.Int) (thor.Bytes32, []*hexutil.Big, []float64, [][]*hexutil.Big, error) {
	newestBlockID := newestBlockSummary.Header.ID()

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)
	var rewards [][]*hexutil.Big
	if rewardPercentiles != nil {
		rewards = make([][]*hexutil.Big, blockCount)
	}

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, err := fd.getOrLoadFees(newestBlockID, rewardPercentiles, baseGasPrice)
		if err != nil {
			// This should happen only when "next" since the boundaries are validated beforehand
			// We do not cache the data in this case since it does not belong to an actual block
			if fd.repo.IsNotFound(err) {
				header := newestBlockSummary.Header
				baseFees[i-1] = getBaseFee(header.BaseFee())
				gasUsedRatios[i-1] = float64(header.GasUsed()) / float64(header.GasLimit())
				if rewardPercentiles != nil {
					//TODO: Use something different here
					blockRewards, err := fd.calculateRewards(nil, rewardPercentiles, baseGasPrice)
					if err != nil {
						return thor.Bytes32{}, nil, nil, nil, err
					}
					rewards[i-1] = blockRewards
				}
				newestBlockID = header.ParentID()
				continue
			}
			return thor.Bytes32{}, nil, nil, nil, err
		}

		baseFees[i-1] = fees.baseFee
		gasUsedRatios[i-1] = fees.gasUsedRatio
		if rewardPercentiles != nil {
			rewards[i-1] = fees.rewards
		}

		newestBlockID = fees.parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, rewards, nil
}

func (fd *FeesData) calculateRewards(block *block.Block, rewardPercentiles *[]float64, baseGasPrice *big.Int) ([]*hexutil.Big, error) {
	// If there is no transactions, return zero rewards
	transactions := block.Transactions()
	if len(transactions) == 0 {
		return make([]*hexutil.Big, len(*rewardPercentiles)), nil
	}

	header := block.Header()
	receipts, err := fd.repo.GetBlockReceipts(header.ID())
	if err != nil {
		return nil, err
	}

	// Calculate rewards
	items := make([]rewardItem, len(transactions))
	isGalactica := header.Number() >= thor.GetForkConfig(fd.repo.NewBestChain().GenesisID()).GALACTICA

	for i, tx := range transactions {
		provedWork, err := tx.ProvedWork(header.Number(), fd.repo.NewBestChain().GetBlockID)
		if err != nil {
			return nil, err
		}
		items[i] = rewardItem{
			reward:  fork.GalacticaPriorityPrice(tx, baseGasPrice, provedWork, &fork.GalacticaItems{IsActive: isGalactica, BaseFee: header.BaseFee()}),
			gasUsed: receipts[i].GasUsed,
		}
	}

	// Sort by reward
	slices.SortStableFunc(items, func(a, b rewardItem) int {
		return a.reward.Cmp(b.reward)
	})

	// Calculate rewards for each percentile
	rewards := make([]*hexutil.Big, len(*rewardPercentiles))
	totalGasUsed := header.GasUsed()

	currentTransactionIndex := 0
	cumulativeGasUsed := items[0].gasUsed

	for i, p := range *rewardPercentiles {
		thresholdGasUsed := uint64(float64(totalGasUsed) * p / 100)
		for cumulativeGasUsed < thresholdGasUsed && currentTransactionIndex < len(transactions)-1 {
			currentTransactionIndex++
			cumulativeGasUsed += items[currentTransactionIndex].gasUsed
		}
		rewards[i] = (*hexutil.Big)(items[currentTransactionIndex].reward)
	}

	return rewards, nil
}

func (fd *FeesData) getOrLoadFees(blockID thor.Bytes32, rewardPercentiles *[]float64, baseGasPrice *big.Int) (*FeeCacheEntry, error) {
	fees, _, found := fd.cache.Get(blockID)
	if found {
		return fees.(*FeeCacheEntry), nil
	}

	block, err := fd.repo.GetBlock(blockID)
	if err != nil {
		return nil, err
	}

	var rewards []*hexutil.Big
	if rewardPercentiles != nil {
		rewards, err = fd.calculateRewards(block, rewardPercentiles, baseGasPrice)
		if err != nil {
			return nil, err
		}
	}

	header := block.Header()
	fees = &FeeCacheEntry{
		baseFee:       getBaseFee(header.BaseFee()),
		gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
		rewards:       rewards,
		parentBlockID: header.ParentID(),
	}
	fd.cache.Set(header.ID(), fees, float64(header.Number()))

	return fees.(*FeeCacheEntry), nil
}
