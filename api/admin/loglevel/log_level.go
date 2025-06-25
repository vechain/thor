// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package loglevel

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/log"
)

type LogLevel struct {
	logLevel *slog.LevelVar
}

func New(logLevel *slog.LevelVar) *LogLevel {
	return &LogLevel{
		logLevel: logLevel,
	}
}

func (l *LogLevel) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").
		Methods(http.MethodGet).
		Name("get-log-level").
		HandlerFunc(utils.WrapHandlerFunc(l.getLogLevelHandler))

	sub.Path("").
		Methods(http.MethodPost).
		Name("post-log-level").
		HandlerFunc(utils.WrapHandlerFunc(l.postLogLevelHandler))
}

func (l *LogLevel) getLogLevelHandler(w http.ResponseWriter, _ *http.Request) error {
	return utils.WriteJSON(w, api.Response{
		CurrentLevel: l.logLevel.Level().String(),
	})
}

func (l *LogLevel) postLogLevelHandler(w http.ResponseWriter, r *http.Request) error {
	var req api.Request

	if err := utils.ParseJSON(r.Body, &req); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "Invalid request body"))
	}

	switch req.Level {
	case "debug":
		l.logLevel.Set(log.LevelDebug)
	case "info":
		l.logLevel.Set(log.LevelInfo)
	case "warn":
		l.logLevel.Set(log.LevelWarn)
	case "error":
		l.logLevel.Set(log.LevelError)
	case "trace":
		l.logLevel.Set(log.LevelTrace)
	case "crit":
		l.logLevel.Set(log.LevelCrit)
	default:
		return utils.BadRequest(errors.New("Invalid verbosity level"))
	}

	log.Info("log level changed", "pkg", "loglevel", "level", l.logLevel.Level().String())

	return utils.WriteJSON(w, api.Response{
		CurrentLevel: l.logLevel.Level().String(),
	})
}
