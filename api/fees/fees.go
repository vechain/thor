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
	// Validate blockCount
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}

	bestBlockSummary := f.data.repo.BestBlockSummary()
	if blockCount == 0 {
		return 0, bestBlockSummary, nil
	}

	// Validate newestBlock
	newestBlock, err := utils.ParseRevisionWithoutBlockID(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}

	newestBlockSummary, err := utils.GetSummary(newestBlock, f.data.repo, f.data.bft)
	if err != nil {
		return 0, nil, err
	}

	// Calculate minAllowedBlock
	minAllowedBlock := uint32(math.Max(0, float64(int(bestBlockSummary.Header.Number())-int(f.data.backtraceLimit)+1)))

	// Adjust blockCount if necessary
	if int(bestBlockSummary.Header.Number()) < int(f.data.backtraceLimit) {
		blockCount = uint64(math.Min(float64(blockCount), float64(bestBlockSummary.Header.Number()+1)))
	}

	if newestBlockSummary.Header.Number() < minAllowedBlock {
		return 0, nil, leveldb.ErrNotFound
	}

	// Adjust blockCount if the oldest block is below the allowed range
	if int(newestBlockSummary.Header.Number())-int(blockCount) < int(minAllowedBlock) {
		blockCount = uint64(newestBlockSummary.Header.Number() - minAllowedBlock + 1)
	}

	return uint32(blockCount), newestBlockSummary, nil
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
