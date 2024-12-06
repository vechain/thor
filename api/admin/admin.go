// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	healthAPI "github.com/vechain/thor/v2/api/admin/health"
	"github.com/vechain/thor/v2/api/admin/loglevel"
)

func New(logLevel *slog.LevelVar, health *healthAPI.Health) http.HandlerFunc {
	router := mux.NewRouter()
	subRouter := router.PathPrefix("/admin").Subrouter()

	loglevel.New(logLevel).Mount(subRouter, "/loglevel")
	healthAPI.NewAPI(health).Mount(subRouter, "/health")

	handler := handlers.CompressHandler(router)

	return handler.ServeHTTP
}
