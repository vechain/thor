// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/health"
)

type Health struct {
	healthStatus *health.Health
}

func New(healthStatus *health.Health) *Health {
	return &Health{
		healthStatus: healthStatus,
	}
}

func (h *Health) handleGetHealth(w http.ResponseWriter, req *http.Request) error {
	acc, err := h.healthStatus.Status()
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, acc)
}

func (h *Health) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/").
		Methods(http.MethodGet).
		Name("health").
		HandlerFunc(utils.WrapHandlerFunc(h.handleGetHealth))
}
