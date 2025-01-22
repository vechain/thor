package fees

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/chain"
)

type Fees struct {
	repo *chain.Repository
}

func New(repo *chain.Repository) *Fees {
	return &Fees{
		repo,
	}
}

func (f *Fees) handlePostFeesHistory(w http.ResponseWriter, req *http.Request) error {
	revision, err := utils.ParseRevision(mux.Vars(req)["revision"], false)
}

func (f *Fees) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/history").
		Methods(http.MethodGet).
		Name("POST /fees/history").
		HandlerFunc(utils.WrapHandlerFunc(f.handlePostFeesHistory))
}