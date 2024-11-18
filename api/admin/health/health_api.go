// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/utils"
)

type API struct {
	healthStatus *Health
}

func NewAPI(healthStatus *Health) *API {
	return &API{
		healthStatus: healthStatus,
	}
}

func (h *API) handleGetHealth(w http.ResponseWriter, _ *http.Request) error {
	acc, err := h.healthStatus.Status()
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
