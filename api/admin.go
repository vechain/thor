// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"log/slog"
	"net/http"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/log"
)

type logLevelRequest struct {
	Level string `json:"level"`
}

type logLevelResponse struct {
	CurrentLevel string `json:"currentLevel"`
}

func getLogLevelHandler(logLevel *slog.LevelVar) utils.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		return utils.WriteJSON(w, logLevelResponse{
			CurrentLevel: logLevel.Level().String(),
		})
	}
}

func postLogLevelHandler(logLevel *slog.LevelVar) utils.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req logLevelRequest

		if err := utils.ParseJSON(r.Body, &req); err != nil {
			return utils.BadRequest(errors.WithMessage(err, "Invalid request body"))
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
			return utils.BadRequest(errors.New("Invalid verbosity level"))
		}

		return utils.WriteJSON(w, logLevelResponse{
			CurrentLevel: logLevel.Level().String(),
		})
	}
}
