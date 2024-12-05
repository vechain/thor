// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/metrics"
)

var (
	metricHTTPReqCounter       = metrics.LazyLoadCounterVec("api_request_count", []string{"name", "code", "method"})
	metricHTTPReqDuration      = metrics.LazyLoadHistogramVec("api_duration_ms", []string{"name", "code", "method"}, metrics.BucketHTTPReqs)
	metricActiveWebsocketCount = metrics.LazyLoadGaugeVec("api_active_websocket_count", []string{"subject"})
	metricTxCallVMErrors       = metrics.LazyLoadCounter("api_tx_call_vm_errors")
)

// metricsResponseWriter is a wrapper around http.ResponseWriter that captures the status code.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{w, http.StatusOK}
}

type callTxResponseWriter struct {
	http.ResponseWriter
	statusCode int
	vmError    bool
}

func newCallTxResponseWriter(w http.ResponseWriter) *callTxResponseWriter {
	return &callTxResponseWriter{w, http.StatusOK, false}
}

func (m *metricsResponseWriter) WriteHeader(code int) {
	m.statusCode = code
	m.ResponseWriter.WriteHeader(code)
}

func (c *callTxResponseWriter) Write(b []byte) (int, error) {
	var resp struct {
		VmError string `json:"vmError"`
	}

	if err := json.Unmarshal(b, &resp); err == nil {
		if resp.VmError != "" {
			c.vmError = true
		}
	}

	return c.ResponseWriter.Write(b)
}

// Hijack complies the writer with WS subscriptions interface
// Hijack lets the caller take over the connection.
// After a call to Hijack the HTTP server library
// will not do anything else with the connection.
//
// It becomes the caller's responsibility to manage
// and close the connection.
func (m *metricsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := m.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

// metricsMiddleware is a middleware that records metrics for each request.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt := mux.CurrentRoute(r)

		var (
			enabled      = false
			name         = ""
			subscription = ""
		)

		if rt != nil && rt.GetName() != "" {
			enabled = true
			name = rt.GetName()

			if name == "transactions_call_tx" {
				ctxWriter := newCallTxResponseWriter(w)
				next.ServeHTTP(ctxWriter, r)

				// Record VM error if present
				if ctxWriter.vmError {
					metricTxCallVMErrors().Add(1)
				}
				return
			}

			// Handle subscriptions
			if strings.HasPrefix(name, "subscriptions") {
				// example path: /subscriptions/txpool -> subject = txpool
				paths := strings.Split(r.URL.Path, "/")
				if len(paths) > 2 {
					subscription = paths[2]
				}
			}
		}

		now := time.Now()
		mrw := newMetricsResponseWriter(w)
		if subscription != "" {
			metricActiveWebsocketCount().AddWithLabel(1, map[string]string{"subject": subscription})
		}

		next.ServeHTTP(mrw, r)

		if subscription != "" {
			metricActiveWebsocketCount().AddWithLabel(-1, map[string]string{"subject": subscription})
		} else if enabled {
			metricHTTPReqCounter().AddWithLabel(1, map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
			metricHTTPReqDuration().ObserveWithLabels(time.Since(now).Milliseconds(), map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
		}
	})
}
