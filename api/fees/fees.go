package fees

import (
	"math/big"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
)

type Fees struct {
	repo *chain.Repository
	bft  bft.Committer
}

func New(repo *chain.Repository, bft bft.Committer) *Fees {
	return &Fees{
		repo,
		bft,
	}
}

func (f *Fees) validateGetFeesHistoryParams(req *http.Request) (uint32, *chain.BlockSummary, error) {
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.ParseUint(blockCountParam, 10, 32)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	if blockCount < 1 || blockCount > maxNumberOfBlocks {
		return 0, nil, utils.BadRequest(errors.New("blockCount must be between 1 and 1024"))
	}
	newestBlockRevision, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}
	newestBlockSummary, err := utils.GetSummary(newestBlockRevision, f.repo, f.bft)
	if err != nil {
		if f.repo.IsNotFound(err) {
			return 0, nil, utils.BadRequest(errors.WithMessage(err, "newestBlock"))
		}
		return 0, nil, err
	}

	return uint32(blockCount), newestBlockSummary, nil
}

func (f *Fees) processBlockRange(blockCount uint32, newestBlockSummary *chain.BlockSummary) (uint32, chan *blockData) {
	lastBlock := newestBlockSummary.Header.Number()
	oldestBlock := lastBlock + 1 - blockCount
	var next atomic.Uint32
	next.Store(oldestBlock)

	blockDataChan := make(chan *blockData, blockCount)

	for i := 0; i < maxBlockFetchers && i < int(blockCount); i++ {
		go func() {
			for {
				// Processing current block and incrementing next block number at the same time
				blockNumber := next.Add(1) - 1
				if blockNumber > lastBlock {
					return
				}
				blockFee := &blockData{}
				blockFee.blockRevision, blockFee.err = utils.ParseRevision(strconv.FormatUint(uint64(blockNumber), 10), false)
				if blockFee.err != nil {
					blockFee.blockSummary, blockFee.err = utils.GetSummary(blockFee.blockRevision, f.repo, f.bft)
				}
				blockDataChan <- blockFee
			}
		}()
	}

	return oldestBlock, blockDataChan
}

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, newestBlockSummary, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		return err
	}
	
	oldestBlock, blockDataChan := f.processBlockRange(blockCount, newestBlockSummary)

	var (
		baseFees      = make([]*big.Int, blockCount)
		gasUsedRatios = make([]float64, blockCount)
	)

	// Collect results from the channel
	for i := 0; i < int(blockCount); i++ {
		blockData := <-blockDataChan
		if blockData.err != nil {
			return blockData.err
		}
		baseFees[i] = blockData.blockSummary.Header.BaseFee()
		gasUsedRatios[i] = float64(blockData.blockSummary.Header.GasUsed()) / float64(blockData.blockSummary.Header.GasLimit())
	}

	return utils.WriteJSON(w, &GetFeesHistory{
		OldestBlock:   &oldestBlock,
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
