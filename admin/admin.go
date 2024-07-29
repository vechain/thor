// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/log"
)

type logLevelRequest struct {
	Level string `json:"level"`
}

type logLevelResponse struct {
	CurrentLevel string `json:"currentLevel"`
}

func getLogLevelHandler(logLevel *slog.LevelVar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := logLevelResponse{
			CurrentLevel: logLevel.Level().String(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}

func postLogLevelHandler(logLevel *slog.LevelVar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req logLevelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		switch req.Level {
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

		fmt.Fprintln(w, "Verbosity changed to ", req.Level)
	}
}

func logLevelHandler(logLevel *slog.LevelVar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getLogLevelHandler(logLevel).ServeHTTP(w, r)
		case http.MethodPost:
			postLogLevelHandler(logLevel).ServeHTTP(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func HTTPHandler(logLevel *slog.LevelVar) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/admin/loglevel", logLevelHandler(logLevel))
	return handlers.CompressHandler(router)
}
