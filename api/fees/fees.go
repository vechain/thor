// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
)

func New(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedCacheSize uint32) *Fees {
	return &Fees{
		data: newFeesData(repo, bft, backtraceLimit, fixedCacheSize),
		done: make(chan struct{}),
	}
}

func getOldestBlockNumber(blockCount uint32, newestBlock uint32) uint32 {
	oldestBlockInt32 := int32(newestBlock) + 1 - int32(blockCount)
	oldestBlock := uint32(0)
	if oldestBlockInt32 >= 0 {
		oldestBlock = uint32(oldestBlockInt32)
	}
	return oldestBlock
}

func (f *Fees) getOldestBlockSummaryByNumber(oldestBlock uint32) (*chain.BlockSummary, error) {
	oldestBlockRevision := utils.NewRevision(oldestBlock)
	oldestBlockSummary, err := utils.GetSummary(oldestBlockRevision, f.data.repo, f.data.bft)
	if err != nil {
		return nil, err
	}
	return oldestBlockSummary, nil
}

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, *chain.BlockSummary, error) {
	//blockCount validation
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	if blockCount < 1 || blockCount > uint64(f.data.size) {
		return 0, nil, nil, utils.BadRequest(errors.New(fmt.Sprintf("blockCount must be between 1 and %d", f.data.size)))
	}

	//newestBlock validation
	newestBlock, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, nil, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}
	// Too new
	newestBlockSummary, err := utils.GetSummary(newestBlock, f.data.repo, f.data.bft)
	if err != nil {
		if f.data.repo.IsNotFound(err) {
			return 0, nil, nil, utils.NotFound(errors.WithMessage(err, "newestBlock"))
		}
		return 0, nil, nil, err
	}
	// Too old
	bestBlockSummary := f.data.repo.BestBlockSummary()
	if bestBlockSummary == nil {
		return 0, nil, nil, utils.HTTPError(errors.New("best block not found"), http.StatusInternalServerError)
	}
	oldestBlockNumberSupported := getOldestBlockNumber(uint32(f.data.size), bestBlockSummary.Header.Number())
	if newestBlockSummary.Header.Number() < oldestBlockNumberSupported {
		return 0, nil, nil, utils.BadRequest(errors.New(fmt.Sprintf("newestBlock must be between %d and %d", oldestBlockNumberSupported, bestBlockSummary.Header.Number())))
	}

	// Get oldest block summary after subtracting blockCount
	// We do not return error, just less blocks, in case this limit goes beyond the backtrace limit
	oldestBlockNumber := getOldestBlockNumber(uint32(blockCount), newestBlockSummary.Header.Number())
	oldestBlockSummary, err := f.getOldestBlockSummaryByNumber(oldestBlockNumber)
	if err != nil {
		return 0, nil, nil, utils.BadRequest(err)
	}

	return uint32(blockCount), oldestBlockSummary, newestBlockSummary, nil
}

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, oldestBlockSummary, newestBlockSummary, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		return err
	}

	cacheOldestBlockNumber, cacheBaseFees, cacheGasUsedRatios, shouldGetBlockSummaries, err := f.data.resolveRange(oldestBlockSummary.Header.ID(), newestBlockSummary.Header.Number())
	if err != nil {
		return utils.HTTPError(err, http.StatusInternalServerError)
	}

	if shouldGetBlockSummaries {
		// Get block summaries for the missing blocks
		newestBlockSummaryNumber := cacheOldestBlockNumber - 1
		summariesGasFees, summariesGasUsedRatios, err := f.data.getBlockSummaries(newestBlockSummaryNumber, blockCount)
		if err != nil {
			return utils.HTTPError(err, http.StatusInternalServerError)
		}
		oldestBlockSummary := oldestBlockSummary.Header.Number()
		return utils.WriteJSON(w, &GetFeesHistory{
			OldestBlock:   &oldestBlockSummary,
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
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	go f.pushBestBlockToCache()
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
