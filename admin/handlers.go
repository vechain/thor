// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/vechain/thor/v2/log"
)

type logLevelRequest struct {
	Level string `json:"level"`
}

type logLevelResponse struct {
	CurrentLevel string `json:"currentLevel"`
}

type errorResponse struct {
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

func writeError(w http.ResponseWriter, errCode int, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(errCode)
	json.NewEncoder(w).Encode(errorResponse{
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
	})
}

func getLogLevelHandler(logLevel *slog.LevelVar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := logLevelResponse{
			CurrentLevel: logLevel.Level().String(),
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to encode response")
		}
	}
}

func postLogLevelHandler(logLevel *slog.LevelVar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req logLevelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
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
			writeError(w, http.StatusBadRequest, "Invalid verbosity level")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		response := logLevelResponse{
			CurrentLevel: logLevel.Level().String(),
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
