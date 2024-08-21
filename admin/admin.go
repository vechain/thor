// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/co"
)

func logLevelHandler(logLevel *slog.LevelVar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getLogLevelHandler(logLevel).ServeHTTP(w, r)
		case http.MethodPost:
			postLogLevelHandler(logLevel).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func HTTPHandler(logLevel *slog.LevelVar) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/admin/loglevel", logLevelHandler(logLevel))
	return handlers.CompressHandler(router)
}

func StartServer(addr string, logLevel *slog.LevelVar) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen admin API addr [%v]", addr)
	}

	router := mux.NewRouter()
	router.PathPrefix("/admin").Handler(HTTPHandler(logLevel))
	handler := handlers.CompressHandler(router)

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var goes co.Goes
	goes.Go(func() {
		srv.Serve(listener)
	})
	return "http://" + listener.Addr().String() + "/admin", func() {
		srv.Close()
		goes.Wait()
	}, nil
}
