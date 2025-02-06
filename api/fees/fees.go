// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
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

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, error) {
	//blockCount validation
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCountUInt64, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	blockCount := uint32(blockCountUInt64)
	if blockCount == 0 {
		return 0, f.data.repo.BestBlockSummary(), nil
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
	minAllowedBlock := f.data.repo.BestBlockSummary().Header.Number() - f.data.backtraceLimit + 1
	if newestBlockSummary.Header.Number() < minAllowedBlock {
		// If the starting block is below the allowed range, return-fast.
		return 0, nil, utils.BadRequest(errors.New("newestBlock not in the allowed range"))
	}

	// If the oldest block is below the allowed range, then adjust blockCount
	if newestBlockSummary.Header.Number()-(blockCount-1) < minAllowedBlock {
		blockCount = newestBlockSummary.Header.Number() - minAllowedBlock + 1
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
	defer f.wg.Done()
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
	go f.pushBestBlockToCache()
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
