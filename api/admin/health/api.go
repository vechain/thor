// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/chain"
)

type API struct {
	health *health
}

func New(repo *chain.Repository, node node.Network, blockInterval time.Duration) *API {
	return &API{health: newHealth(repo, node, blockInterval)}
}

func (h *API) handleGetHealth(w http.ResponseWriter, _ *http.Request) error {
	acc, err := h.health.status()
	if err != nil {
		return err
	}

	if !acc.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable) // Set the status to 503
	} else {
		w.WriteHeader(http.StatusOK) // Set the status to 200
	}
	return utils.WriteJSON(w, acc)
}

func (h *API) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodGet).
		Name("health").
		HandlerFunc(utils.WrapHandlerFunc(h.handleGetHealth))
}
