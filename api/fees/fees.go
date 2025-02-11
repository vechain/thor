// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
)

type Fees struct {
	data *FeesData
	done chan struct{}
	wg   sync.WaitGroup
}
type FeeCacheEntry struct {
	baseFee      *hexutil.Big
	gasUsedRatio float64
}
type FeesData struct {
	repo           *chain.Repository
	cache          *cache.PrioCache
	bft            bft.Committer
	backtraceLimit uint32 // The max number of blocks to backtrace.
}

func New(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedCacheSize uint32) *Fees {
	return &Fees{
		data: newFeesData(repo, bft, backtraceLimit, fixedCacheSize),
		done: make(chan struct{}),
	}
}

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, error) {
	//blockCount validation
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCountUInt64, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	blockCount := uint32(blockCountUInt64)
	bestBlockSummary := f.data.repo.BestBlockSummary()
	if blockCount == 0 {
		return 0, bestBlockSummary, nil
	}

	//newestBlock validation
	newestBlock, err := utils.ParseRevisionWithoutBlockID(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}
	// Too new
	newestBlockSummary, err := utils.GetSummary(newestBlock, f.data.repo, f.data.bft)
	if err != nil {
		return 0, nil, err
	}
	// Too old
	minAllowedBlockInt := int(bestBlockSummary.Header.Number()) - int(f.data.backtraceLimit) + 1
	var minAllowedBlock uint32
	if minAllowedBlockInt < 0 {
		minAllowedBlock = 0
		// If we get to this point, there are less blocks than the backtrace limit.
		// So in case the block count is higher than the number of blocks, we should adjust it.
		blockCount = uint32(math.Min(float64(blockCount), float64(bestBlockSummary.Header.Number()+1)))
	} else {
		minAllowedBlock = uint32(minAllowedBlockInt)
	}

	if newestBlockSummary.Header.Number() < minAllowedBlock {
		// If the starting block is below the allowed range, return-fast.
		return 0, nil, leveldb.ErrNotFound
	}

	// If the oldest block is below the allowed range, then adjust blockCount
	minBlockInt := int(newestBlockSummary.Header.Number()) - int(blockCount) - 1
	if minBlockInt < minAllowedBlockInt {
		blockCount = newestBlockSummary.Header.Number() - minAllowedBlock + 1
	}

	return blockCount, newestBlockSummary, nil
}

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, newestBlockSummary, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		if f.data.repo.IsNotFound(err) {
			// This returns 200 null
			return utils.WriteJSON(w, nil)
		}
		return err
	}

	oldestBlockNumber, baseFees, gasUsedRatios, err := f.data.resolveRange(newestBlockSummary, blockCount)
	if err != nil {
		return utils.HTTPError(err, http.StatusInternalServerError)
	}

	return utils.WriteJSON(w, &FeesHistory{
		OldestBlock:   &oldestBlockNumber,
		BaseFees:      baseFees,
		GasUsedRatios: gasUsedRatios,
	})
}

func (f *Fees) pushBestBlockToCache() {
	ticker := f.data.repo.NewTicker()
	for {
		select {
		case <-ticker.C():
			bestBlockSummary := f.data.repo.BestBlockSummary()
			f.data.pushToCache(bestBlockSummary.Header)
		case <-f.done:
			return
		}
	}
}

func (f *Fees) Close() {
	close(f.done)
	f.wg.Wait()
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.pushBestBlockToCache()
	}()
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
