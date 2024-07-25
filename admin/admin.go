// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/log"
)

func HTTPHandler(logLevel *slog.LevelVar) http.Handler {
	router := mux.NewRouter()
	router.PathPrefix("/admin").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verbosity := r.URL.Query().Get("verbosity")
		switch verbosity {
		case "debug":
			logLevel.Set(log.LevelDebug)
		case "info":
			logLevel.Set(log.LevelInfo)
		case "warn":
			logLevel.Set(log.LevelWarn)
		case "error":
			logLevel.Set(log.LevelError)
		case "trace":
			logLevel.Set(log.LevelTrace)
		case "crit":
			logLevel.Set(log.LevelCrit)
		default:
			http.Error(w, "Invalid verbosity level", http.StatusBadRequest)
			return
		}

		fmt.Fprintln(w, "Verbosity changed to ", verbosity)
	}))
	return handlers.CompressHandler(router)
}
