// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"log/slog"
	"mime"
	"net/http"
	"sync/atomic"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/admin/featuregate"
	"github.com/vechain/thor/v2/api/admin/loglevel"
	"github.com/vechain/thor/v2/api/admin/pprof"
	"github.com/vechain/thor/v2/cmd/thor/node"

	healthAPI "github.com/vechain/thor/v2/api/admin/health"
)

func NewHTTPHandler(
	logLevel *slog.LevelVar,
	health *healthAPI.Health,
	apiLogsGate *featuregate.Gate,
	txpoolAPIGate *featuregate.Gate,
	pprofGate *featuregate.Gate,
	master *node.Master,
) http.HandlerFunc {
	router := mux.NewRouter()
	subRouter := router.PathPrefix("/admin").Subrouter()
	subRouter.Use(requireJSONContentType)

	loglevel.New(logLevel).Mount(subRouter, "/loglevel")
	healthAPI.NewAPI(health, master).Mount(subRouter, "/health")

	reg := featuregate.NewRegistry()
	reg.Add(apiLogsGate)
	reg.Add(txpoolAPIGate)
	reg.Add(pprofGate)
	reg.MountAPI(subRouter, "/features")

	// Legacy alias — /admin/apilogs predates the unified /admin/features
	// namespace; kept for backward compatibility with existing clients.
	reg.MountLegacyAlias(subRouter, "/apilogs", "apilogs")

	// pprof's /debug/pprof/* handlers must live on the router root
	// (net/http/pprof.Index hard-codes the prefix). Gated by pprofGate.
	pprof.MountHandlers(router, pprofGate)

	handler := handlers.CompressHandler(router)
	return handler.ServeHTTP
}

// requireJSONContentType rejects state-changing requests whose Content-Type is
// not application/json. HTML forms and CORS "simple requests" cannot send that
// type, and a JSON content-type forces a preflight the admin server never
// answers, so this closes the browser path for cross-site form posts.
func requireJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// NewGate builds a featuregate.Gate pre-wired with the admin audit metric.
// Callers don't need to know about the metric layer; this keeps the
// "every admin toggle is audited" invariant inside this package.
func NewGate(name string, enabled *atomic.Bool) *featuregate.Gate {
	return featuregate.New(name, enabled, recordToggle)
}
