// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package apilogs

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/log"
)

type APILogs struct {
	enabled *atomic.Bool
	mu      sync.Mutex
}

func New(enabled *atomic.Bool) *APILogs {
	return &APILogs{
		enabled: enabled,
	}
}

func (a *APILogs) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").
		Methods(http.MethodGet).
		Name("get-api-logs-enabled").
		HandlerFunc(utils.WrapHandlerFunc(a.areAPILogsEnabled))

	sub.Path("").
		Methods(http.MethodPost).
		Name("post-api-logs-enabled").
		HandlerFunc(utils.WrapHandlerFunc(a.setAPILogsEnabled))
}

func (a *APILogs) areAPILogsEnabled(w http.ResponseWriter, _ *http.Request) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	return utils.WriteJSON(w, api.LogStatus{
		Enabled: a.enabled.Load(),
	})
}

func (a *APILogs) setAPILogsEnabled(w http.ResponseWriter, r *http.Request) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var req api.LogStatus
	if err := utils.ParseJSON(r.Body, &req); err != nil {
		return utils.BadRequest(err)
	}
	a.enabled.Store(req.Enabled)

	log.Info("api logs updated", "pkg", "apilogs", "enabled", req.Enabled)

	return utils.WriteJSON(w, api.LogStatus{
		Enabled: a.enabled.Load(),
	})
}
