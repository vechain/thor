// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
)

type Fees struct {
	data           *FeesData
	bft            bft.Committer
	backtraceLimit uint32 // The max number of blocks to backtrace.
}

func New(repo *chain.Repository, bft bft.Committer, backtraceLimit uint32, fixedCacheSize uint32) *Fees {
	return &Fees{
		data:           newFeesData(repo, fixedCacheSize),
		bft:            bft,
		backtraceLimit: backtraceLimit,
	}
}

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, error) {
	// Validate blockCount
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}

	if blockCount == 0 {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should not be 0"))
	}

	// Validate newestBlock
	newestBlock, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}

	newestBlockSummary, err := utils.GetSummary(newestBlock, f.data.repo, f.bft)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}

	bestBlockNumber := f.data.repo.BestBlockSummary().Header.Number()
	// Calculate minAllowedBlock
	minAllowedBlock := uint32(math.Max(0, float64(int(bestBlockNumber)-int(f.backtraceLimit)+1)))

	// Adjust blockCount if necessary
	if int(bestBlockNumber) < int(f.backtraceLimit) {
		blockCount = uint64(math.Min(float64(blockCount), float64(bestBlockNumber+1)))
	}

	if newestBlockSummary.Header.Number() < minAllowedBlock {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid newestBlock, it is below the minimum allowed block"))
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
		return err
	}

	oldestBlockRevision, baseFees, gasUsedRatios, err := f.data.resolveRange(newestBlockSummary, blockCount)
	if err != nil {
		return utils.HTTPError(err, http.StatusInternalServerError)
	}

	return utils.WriteJSON(w, &FeesHistory{
		OldestBlock:   oldestBlockRevision,
		BaseFees:      baseFees,
		GasUsedRatios: gasUsedRatios,
	})
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
