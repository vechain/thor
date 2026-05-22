// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/admin/featuregate"
	"github.com/vechain/thor/v2/api/admin/loglevel"
	"github.com/vechain/thor/v2/cmd/thor/node"

	healthAPI "github.com/vechain/thor/v2/api/admin/health"
)

func NewHTTPHandler(
	logLevel *slog.LevelVar,
	health *healthAPI.Health,
	apiLogsGate *featuregate.Gate,
	txpoolAPIGate *featuregate.Gate,
	master *node.Master,
) http.HandlerFunc {
	router := mux.NewRouter()
	subRouter := router.PathPrefix("/admin").Subrouter()

	loglevel.New(logLevel).Mount(subRouter, "/loglevel")
	healthAPI.NewAPI(health, master).Mount(subRouter, "/health")

	reg := featuregate.NewRegistry()
	reg.Add(apiLogsGate)
	reg.Add(txpoolAPIGate)
	reg.MountAPI(subRouter, "/features")

	// Legacy alias — /admin/apilogs predates the unified /admin/features
	// namespace; kept for backward compatibility with existing clients.
	reg.MountLegacyAlias(subRouter, "/apilogs", "apilogs")

	handler := handlers.CompressHandler(router)
	return handler.ServeHTTP
}

// NewGate builds a featuregate.Gate pre-wired with the admin audit metric.
// Callers don't need to know about the metric layer; this keeps the
// "every admin toggle is audited" invariant inside this package.
func NewGate(name string, enabled *atomic.Bool) *featuregate.Gate {
	return featuregate.New(name, enabled, recordToggle)
}
