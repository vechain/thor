// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/log"
)

type Admin struct {
	address     string
	logLevel    *slog.LevelVar
	logRequests *atomic.Bool
}

func NewAdmin(addr string, logLevel *slog.LevelVar, logRequests *atomic.Bool) *Admin {
	return &Admin{
		address:     addr,
		logLevel:    logLevel,
		logRequests: logRequests,
	}
}

// Start the admin server.
func (a *Admin) Start() (string, func(), error) {
	listener, err := net.Listen("tcp", a.address)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen admin API addr [%v]", a.address)
	}

	router := mux.NewRouter()
	handler := handlers.CompressHandler(router)
	sub := router.PathPrefix("/admin").Subrouter()

	// GET /admin/loglevel
	sub.Path("/loglevel").
		Methods(http.MethodGet).
		Name("get-log-level").
		HandlerFunc(utils.WrapHandlerFunc(a.getLogLevelHandler))
	// POST /admin/loglevel
	sub.Path("/loglevel").
		Methods(http.MethodPost).
		Name("post-log-level").
		HandlerFunc(utils.WrapHandlerFunc(a.postLogLevelHandler))

	// GET /admin/apilogs
	sub.Path("/apilogs").
		Methods(http.MethodGet).
		Name("get-api-logs-enabled").
		Handler(utils.WrapHandlerFunc(a.getRequestLoggerEnabled))
	// POST /admin/apilogs
	sub.Path("/apilogs").
		Methods(http.MethodPost).
		Name("post-api-logs-enabled").
		Handler(utils.WrapHandlerFunc(a.postRequestLogger))

	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var goes co.Goes
	goes.Go(func() {
		server.Serve(listener)
	})

	cancel := func() {
		server.Close()
		goes.Wait()
	}

	return "http://" + listener.Addr().String() + "/admin", cancel, nil
}

type logLevelRequest struct {
	Level string `json:"level"`
}

type logLevelResponse struct {
	CurrentLevel string `json:"currentLevel"`
}

func (a *Admin) getLogLevelHandler(w http.ResponseWriter, r *http.Request) error {
	return utils.WriteJSON(w, logLevelResponse{
		CurrentLevel: a.logLevel.Level().String(),
	})
}

func (a *Admin) postLogLevelHandler(w http.ResponseWriter, r *http.Request) error {
	var req logLevelRequest

	if err := utils.ParseJSON(r.Body, &req); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "invalid request body"))
	}

	switch req.Level {
	case "debug":
		a.logLevel.Set(log.LevelDebug)
	case "info":
		a.logLevel.Set(log.LevelInfo)
	case "warn":
		a.logLevel.Set(log.LevelWarn)
	case "error":
		a.logLevel.Set(log.LevelError)
	case "trace":
		a.logLevel.Set(log.LevelTrace)
	case "crit":
		a.logLevel.Set(log.LevelCrit)
	default:
		return utils.BadRequest(fmt.Errorf("invalid verbosity level: %s", req.Level))
	}

	log.Warn("admin changed the log level", "level", log.LevelString(a.logLevel.Level()))

	return utils.WriteJSON(w, logLevelResponse{
		CurrentLevel: a.logLevel.Level().String(),
	})
}

type apiLogRequests struct {
	Enabled *bool `json:"enabled"`
}

func (a *Admin) getRequestLoggerEnabled(w http.ResponseWriter, r *http.Request) error {
	enabled := a.logRequests.Load()
	res := apiLogRequests{
		Enabled: &enabled,
	}
	return utils.WriteJSON(w, res)
}

func (a *Admin) postRequestLogger(w http.ResponseWriter, r *http.Request) error {
	var req apiLogRequests

	if err := utils.ParseJSON(r.Body, &req); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "invalid request body"))
	}

	if req.Enabled == nil {
		return utils.BadRequest(errors.New("missing 'enabled' field"))
	}

	log.Warn("admin changed the request logger", "enabled", *req.Enabled)

	a.logRequests.Store(*req.Enabled)

	return utils.WriteJSON(w, req)
}
