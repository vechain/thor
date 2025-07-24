// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"net"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/metrics"
)

func StartMetricsServer(addr string) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen metrics API addr [%v]", addr)
	}

	router := mux.NewRouter()
	router.PathPrefix("/metrics").Handler(metrics.HTTPHandler())
	handler := handlers.CompressHandler(router)

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var goes co.Goes
	goes.Go(func() {
		srv.Serve(listener)
	})
	return "http://" + listener.Addr().String() + "/metrics", func() {
		srv.Close()
		goes.Wait()
	}, nil
}
