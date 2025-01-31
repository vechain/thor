// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

func New(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedCacheSize uint32) *Fees {
	return &Fees{
		repo:  repo,
		bft:   bft,
		cache: NewFeesCache(repo, backtraceLimit, fixedCacheSize),
		done:  make(chan struct{}),
	}
}

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, uint32, error) {
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, 0, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	if blockCount < 1 || blockCount > uint64(f.cache.size) {
		return 0, 0, utils.BadRequest(errors.New(fmt.Sprintf("blockCount must be between 1 and %d", f.cache.size)))
	}
	newestBlock, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, 0, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}

	// Newest block should always be in the cache
	_, newestBlockNumber, err := f.cache.getByRevision(newestBlock, f.bft)
	if err != nil {
		return 0, 0, utils.NotFound(errors.WithMessage(err, "newestBlock"))
	}

	return uint32(blockCount), newestBlockNumber, nil
}

func getOldestBlockNumber(blockCount uint32, newestBlock uint32) uint32 {
	oldestBlockInt32 := int32(newestBlock) + 1 - int32(blockCount)
	oldestBlock := uint32(0)
	if oldestBlockInt32 >= 0 {
		oldestBlock = uint32(oldestBlockInt32)
	}
	return oldestBlock
}

func (f *Fees) getOldestBlockIDByNumber(oldestBlock uint32) (thor.Bytes32, error) {
	oldestBlockRevision := utils.ParseNumberRevision(oldestBlock)
	oldestFeeCacheEntry, err := utils.ParseBlockID(oldestBlockRevision, f.cache.repo, f.bft)
	if err != nil {
		return thor.Bytes32{}, err
	}
	return oldestFeeCacheEntry, nil
}

func (f *Fees) processBlockSummaries(next *atomic.Uint32, lastBlock uint32, blockDataChan chan *blockData) {
	for {
		// Processing current block and incrementing next block number at the same time
		blockNumber := next.Add(1) - 1
		if blockNumber > lastBlock {
			return
		}
		blockFee := &blockData{}
		blockFee.blockRevision, blockFee.err = utils.ParseRevision(strconv.FormatUint(uint64(blockNumber), 10), false)
		if blockFee.err == nil {
			blockFee.blockSummary, blockFee.err = utils.GetSummary(blockFee.blockRevision, f.repo, f.bft)
			if blockFee.blockSummary == nil {
				blockFee.err = fmt.Errorf("block summary is nil for block number %d", blockNumber)
			}
		}
		blockDataChan <- blockFee
	}
}

func (f *Fees) processBlockRange(blockCount uint32, summary *chain.BlockSummary) (uint32, chan *blockData) {
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
			f.processBlockSummaries(&next, lastBlock, blockDataChan)
		}()
	}

	go func() {
		wg.Wait()
		close(blockDataChan)
	}()

	return oldestBlock, blockDataChan
}

func (f *Fees) getBlockSummaries(newestBlockSummaryNumber uint32, blockCount uint32) ([]*hexutil.Big, []float64, error) {
	newestBlockRevision := utils.ParseNumberRevision(newestBlockSummaryNumber)
	summary, err := utils.GetSummary(newestBlockRevision, f.repo, f.bft)
	if err != nil {
		return nil, nil, err
	}

	oldestBlock, blockDataChan := f.processBlockRange(f.cache.backtraceLimit-f.cache.fixedSize, summary)

	var (
		baseFeesWithNil = make([]*hexutil.Big, blockCount)
		gasUsedRatios   = make([]float64, blockCount)
	)

	// Collect results from the channel
	for blockData := range blockDataChan {
		if blockData.err != nil {
			return nil, nil, blockData.err
		}
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

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, newestBlockNumber, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		return err
	}

	oldestBlockNumber := getOldestBlockNumber(blockCount, newestBlockNumber)
	oldestBlockID, err := f.getOldestBlockIDByNumber(oldestBlockNumber)
	if err != nil {
		return utils.BadRequest(err)
	}

	cacheOldestBlockNumber, cacheBaseFees, cacheGasUsedRatios, shouldGetBlockSummaries, err := f.cache.resolveRange(oldestBlockID, newestBlockNumber)
	if err != nil {
		return utils.HTTPError(err, http.StatusInternalServerError)
	}

	if shouldGetBlockSummaries {
		// Get block summaries for the missing blocks
		newestBlockSummaryNumber := cacheOldestBlockNumber - 1
		summariesGasFees, summariesGasUsedRatios, err := f.getBlockSummaries(newestBlockSummaryNumber, blockCount)
		if err != nil {
			return utils.HTTPError(err, http.StatusInternalServerError)
		}
		return utils.WriteJSON(w, &GetFeesHistory{
			OldestBlock:   &oldestBlockNumber,
			BaseFees:      append(summariesGasFees, cacheBaseFees...),
			GasUsedRatios: append(summariesGasUsedRatios, cacheGasUsedRatios...),
		})
	} else {
		return utils.WriteJSON(w, &GetFeesHistory{
			OldestBlock:   &cacheOldestBlockNumber,
			BaseFees:      cacheBaseFees,
			GasUsedRatios: cacheGasUsedRatios,
		})
	}
}

func (f *Fees) pushBestBlockToCache() {
	ticker := f.repo.NewTicker()
	for {
		select {
		case <-ticker.C():
			bestBlockSummary := f.repo.BestBlockSummary()
			f.cache.Push(bestBlockSummary.Header)
		case <-f.done:
			return
		}
	}
}

func (f *Fees) Close() {
	close(f.done)
}

func (f *Fees) CacheLen() int {
	return f.cache.cache.Len()
}

func (f *Fees) CachePriorities() []float64 {
	priorities := make([]float64, 0, f.cache.cache.Len())
	f.cache.cache.ForEach(func(ent *cache.PrioEntry) bool {
		priorities = append(priorities, ent.Priority)
		return true
	})
	return priorities
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	go f.pushBestBlockToCache()
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
