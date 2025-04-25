// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

const maxRewardPercentiles = 100

type Config struct {
	APIBacktraceLimit          int
	PriorityIncreasePercentage int
	FixedCacheSize             int
}

type Fees struct {
	data           *FeesData
	bft            bft.Committer
	minPriorityFee *big.Int // The minimum suggested priority fee is (Config.PriorityIncreasePercentage)% of the block initial base fee.
	config         Config
}

func calcPriorityFee(baseFee *big.Int, priorityPercentile int64) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(baseFee, big.NewInt(priorityPercentile)), big.NewInt(100))
}

func New(repo *chain.Repository, bft bft.Committer, stater *state.Stater, config Config) *Fees {
	return &Fees{
		data:           newFeesData(repo, stater, config.FixedCacheSize),
		bft:            bft,
		minPriorityFee: calcPriorityFee(big.NewInt(thor.InitialBaseFee), int64(config.PriorityIncreasePercentage)),
		config:         config,
	}
}

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, []float64, error) {
	blockCount, err := f.validateBlockCount(req)
	if err != nil {
		return 0, nil, nil, err
	}

	newestBlockSummary, adjustedBlockCount, err := f.validateNewestBlock(req, blockCount)
	if err != nil {
		return 0, nil, nil, err
	}

	rewardPercentiles, err := f.validateRewardPercentiles(req)
	if err != nil {
		return 0, nil, nil, err
	}

	return uint32(adjustedBlockCount), newestBlockSummary, rewardPercentiles, nil
}

func (f *Fees) validateBlockCount(req *http.Request) (uint64, error) {
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}

	if blockCount == 0 {
		return 0, utils.BadRequest(errors.New("invalid blockCount, it should not be 0"))
	}

	return blockCount, nil
}

func (f *Fees) validateNewestBlock(req *http.Request, blockCount uint64) (*chain.BlockSummary, uint64, error) {
	newestBlock, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), true)
	if err != nil {
		return nil, 0, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}

	newestBlockSummary, _, err := utils.GetSummaryAndState(newestBlock, f.data.repo, f.bft, f.data.stater)
	if err != nil {
		if f.data.repo.IsNotFound(err) {
			return nil, 0, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
		}
		return nil, 0, err
	}

	bestBlockNumber := f.data.repo.BestBlockSummary().Header.Number()
	minAllowedBlock := uint32(math.Max(0, float64(int(bestBlockNumber)-f.config.APIBacktraceLimit+1)))

	if newestBlockSummary.Header.Number() < minAllowedBlock {
		return nil, 0, utils.BadRequest(errors.New("invalid newestBlock, it is below the minimum allowed block"))
	}

	adjustedBlockCount := blockCount
	if int(newestBlockSummary.Header.Number())-int(adjustedBlockCount) < int(minAllowedBlock) {
		adjustedBlockCount = uint64(newestBlockSummary.Header.Number() - minAllowedBlock + 1)
	}

	return newestBlockSummary, adjustedBlockCount, nil
}

func (f *Fees) validateRewardPercentiles(req *http.Request) ([]float64, error) {
	rewardPercentilesParam := req.URL.Query().Get("rewardPercentiles")
	if rewardPercentilesParam == "" {
		return nil, nil
	}

	percentileStrs := strings.Split(rewardPercentilesParam, ",")
	rewardPercentiles := make([]float64, 0, len(percentileStrs))

	if len(percentileStrs) > maxRewardPercentiles {
		return nil, utils.BadRequest(errors.New(fmt.Sprintf("there can be at most %d rewardPercentiles", maxRewardPercentiles)))
	}

	for i, str := range percentileStrs {
		val, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return nil, utils.BadRequest(errors.WithMessage(err, "invalid rewardPercentiles value"))
		}
		if val < 0 || val > 100 {
			return nil, utils.BadRequest(errors.New("rewardPercentiles values must be between 0 and 100"))
		}
		if i > 0 && val < rewardPercentiles[i-1] {
			return nil, utils.BadRequest(errors.New(fmt.Sprintf("reward percentiles must be in ascending order, but %f is less than %f", val, rewardPercentiles[i-1])))
		}
		rewardPercentiles = append(rewardPercentiles, val)
	}

	return rewardPercentiles, nil
}

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, newestBlockSummary, rewardPercentiles, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		return err
	}

	oldestBlockRevision, baseFees, gasUsedRatios, rewards, err := f.data.resolveRange(newestBlockSummary, blockCount, rewardPercentiles)
	if err != nil {
		return err
	}

	return utils.WriteJSON(w, &FeesHistory{
		OldestBlock:   oldestBlockRevision,
		BaseFeePerGas: baseFees,
		GasUsedRatio:  gasUsedRatios,
		Reward:        rewards,
	})
}

func (f *Fees) handleGetPriority(w http.ResponseWriter, _ *http.Request) error {
	bestBlockSummary := f.data.repo.BestBlockSummary()

	priorityFee := (*hexutil.Big)(f.minPriorityFee)
	if bestBlockSummary.Header.BaseFee() != nil {
		forkConfig := thor.GetForkConfig(f.data.repo.NewBestChain().GenesisID())
		nextBaseFee := galactica.CalcBaseFee(bestBlockSummary.Header, forkConfig)
		if nextBaseFee.Cmp(big.NewInt(thor.InitialBaseFee)) > 0 {
			priorityFee = (*hexutil.Big)(calcPriorityFee(nextBaseFee, int64(f.config.PriorityIncreasePercentage)))
		}
	}

	return utils.WriteJSON(w, &FeesPriority{
		MaxPriorityFeePerGas: priorityFee,
	})
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
	sub.Path("/priority").
		Methods(http.MethodGet).
		Name("GET /fees/priority").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetPriority))
}
