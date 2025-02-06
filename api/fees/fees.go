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

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, error) {
	//blockCount validation
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCountUInt64, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	blockCount := uint32(blockCountUInt64)
	if blockCount < 1 || blockCount > f.data.maxBacktraceLimit {
		return 0, nil, utils.BadRequest(errors.New(fmt.Sprintf("blockCount must be between 1 and %d", f.data.maxBacktraceLimit)))
	}

	//newestBlock validation
	newestBlock, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}
	// Too new
	newestBlockSummary, err := utils.GetSummary(newestBlock, f.data.repo, f.data.bft)
	if err != nil {
		return 0, nil, err
	}
	// Too old
	bestBlockSummary := f.data.repo.BestBlockSummary()
	newestBlockNumberSupported := getOldestBlockNumber(f.data.cacheSize, bestBlockSummary.Header.Number())
	if newestBlockNumberSupported > newestBlockSummary.Header.Number() {
		return 0, nil, utils.BadRequest(errors.New(fmt.Sprintf("newestBlock must be between %d and %d", newestBlockNumberSupported, bestBlockSummary.Header.Number())))
	}

	// Get oldest block summary after subtracting blockCount
	// We do not return error, just less blocks, in case this limit goes beyond the backtrace limit
	oldestBlockNumber := getOldestBlockNumber(blockCount, newestBlockSummary.Header.Number())
	oldestBlockNumberSupported := getOldestBlockNumber(f.data.maxBacktraceLimit, bestBlockSummary.Header.Number())
	if oldestBlockNumberSupported > oldestBlockNumber {
		blockCount = newestBlockSummary.Header.Number() - oldestBlockNumber + 1
	}

	return blockCount, newestBlockSummary, nil
}

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, newestBlockSummary, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		if f.data.repo.IsNotFound(err) {
			return utils.WriteJSON(w, nil)
		}
		return err
	}

	oldestBlockNumber, baseFees, gasUsedRatios, err := f.data.resolveRange(newestBlockSummary, blockCount)
	if err != nil {
		return utils.HTTPError(err, http.StatusInternalServerError)
	}

	return utils.WriteJSON(w, &GetFeesHistory{
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
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	go f.pushBestBlockToCache()
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
