package fees

import (
	"net/http"

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

	baseFee := summary.Header.BaseFee()

	return utils.WriteJSON(w, baseFee)
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("GET /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handleGetFeesHistory))
}
