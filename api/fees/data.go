// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math/big"

	"slices"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type FeesData struct {
	repo   *chain.Repository
	cache  *cache.PrioCache
	stater *state.Stater
}

type rewardItem struct {
	reward  *big.Int
	gasUsed uint64
}

type rewards struct {
	items        []rewardItem
	totalGasUsed uint64
}

type FeeCacheEntry struct {
	parentBlockID thor.Bytes32
	baseFee       *hexutil.Big
	gasUsedRatio  float64
	cachedRewards *rewards
}

func newFeesData(repo *chain.Repository, stater *state.Stater, fixedCacheSize int) *FeesData {
	return &FeesData{
		repo:   repo,
		stater: stater,
		cache:  cache.NewPrioCache(fixedCacheSize),
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
func (fd *FeesData) resolveRange(newestBlockSummary *chain.BlockSummary, blockCount uint32, rewardPercentiles []float64) (thor.Bytes32, []*hexutil.Big, []float64, [][]*hexutil.Big, error) {
	newestBlockID := newestBlockSummary.Header.ID()

	baseFees := make([]*hexutil.Big, blockCount)
	gasUsedRatios := make([]float64, blockCount)
	var rewards [][]*hexutil.Big
	if len(rewardPercentiles) > 0 {
		rewards = make([][]*hexutil.Big, blockCount)
	}

	var oldestBlockID thor.Bytes32
	for i := blockCount; i > 0; i-- {
		oldestBlockID = newestBlockID
		fees, err := fd.getOrLoadFees(newestBlockID, rewardPercentiles)
		if err != nil {
			// This should happen only when "next" since the boundaries are validated beforehand
			// We do not cache the data in this case since it does not belong to an actual block
			if fd.repo.IsNotFound(err) {
				header := newestBlockSummary.Header
				baseFees[i-1] = getBaseFee(header.BaseFee())
				if rewards != nil {
					// There are no transactions in this block, so the rewards are empty
					rewards[i-1] = fd.calculateRewards(nil, rewardPercentiles)
				}
				newestBlockID = header.ParentID()
				continue
			}
			return thor.Bytes32{}, nil, nil, nil, err
		}

		baseFees[i-1] = fees.baseFee
		gasUsedRatios[i-1] = fees.gasUsedRatio
		if rewards != nil {
			rewards[i-1] = fd.calculateRewards(fees.cachedRewards, rewardPercentiles)
		}

		newestBlockID = fees.parentBlockID
	}

	return oldestBlockID, baseFees, gasUsedRatios, rewards, nil
}

func (fd *FeesData) getOrLoadFees(blockID thor.Bytes32, rewardPercentiles []float64) (*FeeCacheEntry, error) {
	fees, _, found := fd.cache.Get(blockID)
	if found {
		return fees.(*FeeCacheEntry), nil
	}

	block, err := fd.repo.GetBlock(blockID)
	if err != nil {
		return nil, err
	}

	var rewards *rewards
	// If rewardPercentiles is not empty, we need to calculate the rewards for the block
	if len(rewardPercentiles) > 0 {
		rewards, err = fd.getRewardsForCache(block)
		if err != nil {
			return nil, err
		}
	}

	header := block.Header()
	fees = &FeeCacheEntry{
		baseFee:       getBaseFee(header.BaseFee()),
		gasUsedRatio:  float64(header.GasUsed()) / float64(header.GasLimit()),
		cachedRewards: rewards,
		parentBlockID: header.ParentID(),
	}
	fd.cache.Set(header.ID(), fees, float64(header.Number()))

	return fees.(*FeeCacheEntry), nil
}

func (fd *FeesData) getRewardsForCache(block *block.Block) (*rewards, error) {
	header := block.Header()
	receipts, err := fd.repo.GetBlockReceipts(header.ID())
	if err != nil {
		return nil, err
	}

	// Get the baseGasPrice for legacy transactions
	parentSummary, err := fd.repo.GetBlockSummary(block.Header().ParentID())
	if err != nil {
		return nil, err
	}
	state := fd.stater.NewState(parentSummary.Root())

	baseGasPrice, err := builtin.Params.Native(state).Get(thor.KeyBaseGasPrice)
	if err != nil {
		return nil, err
	}

	// Get the effective priority fee (reward) for each transaction
	transactions := block.Transactions()
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

	return &rewards{
		items:        items,
		totalGasUsed: header.GasUsed(),
	}, nil
}

func (fd *FeesData) calculateRewards(cachedRewards *rewards, rewardPercentiles []float64) []*hexutil.Big {
	rewards := make([]*hexutil.Big, len(rewardPercentiles))
	// If there are no reward items, return rewards with value 0
	if cachedRewards == nil || len(cachedRewards.items) == 0 {
		for i := range rewards {
			rewards[i] = (*hexutil.Big)(big.NewInt(0))
		}
		return rewards
	}

	currentTransactionIndex := 0
	cumulativeGasUsed := cachedRewards.items[0].gasUsed

	// For each percentile, we calculate the cumulative gas used until reaching the threshold
	// The threshold is calculated as: (total gas used * percentile) / 100
	// For example, if total gas was 1000 and percentile is 50, the threshold is 500
	// We iterate over transactions sorted by reward until we exceed the threshold
	// or reach the last transaction
	for i, p := range rewardPercentiles {
		thresholdGasUsed := uint64(float64(cachedRewards.totalGasUsed) * p / 100)
		for cumulativeGasUsed < thresholdGasUsed && currentTransactionIndex < len(cachedRewards.items)-1 {
			currentTransactionIndex++
			cumulativeGasUsed += cachedRewards.items[currentTransactionIndex].gasUsed
		}
		rewards[i] = (*hexutil.Big)(cachedRewards.items[currentTransactionIndex].reward)
	}

	return rewards
}
