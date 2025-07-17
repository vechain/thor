// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/restutil"
)

type API struct {
	healthStatus *Health
}

func NewAPI(healthStatus *Health) *API {
	return &API{
		healthStatus: healthStatus,
	}
}

func (h *API) handleGetHealth(w http.ResponseWriter, r *http.Request) error {
	// Parse query parameters
	query := r.URL.Query()

	// Default to constants if query parameters are not provided
	blockTolerance := defaultBlockTolerance
	minPeerCount := defaultMinPeerCount

	// Override with query parameters if they exist
	if queryBlockTolerance := query.Get("blockTolerance"); queryBlockTolerance != "" {
		if parsed, err := time.ParseDuration(queryBlockTolerance); err == nil {
			blockTolerance = parsed
		}
	}

	if queryMinPeerCount := query.Get("minPeerCount"); queryMinPeerCount != "" {
		if parsed, err := strconv.Atoi(queryMinPeerCount); err == nil {
			minPeerCount = parsed
		}
	}

	acc, err := h.healthStatus.Status(blockTolerance, minPeerCount)
	if err != nil {
		return err
	}

	if !acc.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable) // Set the status to 503
	} else {
		w.WriteHeader(http.StatusOK) // Set the status to 200
	}
	return restutil.WriteJSON(w, acc)
}

func (h *API) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodGet).
		Name("health").
		HandlerFunc(restutil.WrapHandlerFunc(h.handleGetHealth))
}
