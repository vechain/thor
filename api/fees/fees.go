package fees

import (
	"math/big"
	"net/http"
	"strconv"

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

func (f *Fees) handleGetFeesHistory(w http.ResponseWriter, req *http.Request) error {
	blockCountParam := req.URL.Query().Get("blockCount")
	blockCount, err := strconv.Atoi(blockCountParam)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "invalid blockCount, it should represent an integer"))
	}
	if blockCount < 1 || blockCount > 1024 {
		return utils.BadRequest(errors.New("blockCount must be between 1 and 1024"))
	}
	newestBlock, err := utils.ParseRevision(req.URL.Query().Get("newestBlock"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "newestBlock"))
	}
	//TODO: get the array of fees from the newest block to the blockCount
	summary, err := utils.GetSummary(newestBlock, f.repo, f.bft)
	if err != nil {
		if f.repo.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "newestBlock"))
		}
		return err
	}

	baseFees := make([]*big.Int, blockCount)
	baseFees[0] = summary.Header.BaseFee()

	return utils.WriteJSON(w, &GetFeesHistory{
		OldestBlock:  nil,
		BaseFees:     baseFees,
		GasUsedRatio: nil,
	})
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
