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
		return 0, nil, utils.BadRequest(errors.New(fmt.Sprintf("blockCount must be between 1 and %d", maxNumberOfBlocks)))
	}
	revision, err := utils.ParseRevision(req.URL.Query().Get("revision"), false)
	if err != nil {
		return 0, nil, utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	summary, err := utils.GetSummary(revision, f.repo, f.bft)
	if err != nil {
		if f.repo.IsNotFound(err) {
			return 0, nil, utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return 0, nil, err
	}

	return uint32(blockCount), summary, nil
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

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCount, summary, err := f.validateGetFeesHistoryParams(req)
	if err != nil {
		return err
	}

	oldestBlock, blockDataChan := f.processBlockRange(blockCount, summary)
	numberOfProcessedBlocks := len(blockDataChan)

	var (
		baseFees      = make([]*hexutil.Big, numberOfProcessedBlocks)
		gasUsedRatios = make([]float64, numberOfProcessedBlocks)
	)

	// Collect results from the channel
	for blockData := range blockDataChan {
		if blockData.err != nil {
			return blockData.err
		}
		if baseFee := blockData.blockSummary.Header.BaseFee(); baseFee != nil {
			baseFees = append(baseFees, (*hexutil.Big)(baseFee))
		} else {
			baseFees = append(baseFees, (*hexutil.Big)(big.NewInt(0)))
		}
		gasUsedRatios = append(gasUsedRatios, float64(blockData.blockSummary.Header.GasUsed())/float64(blockData.blockSummary.Header.GasLimit()))
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
