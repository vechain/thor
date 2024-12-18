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
	websocketDurations = []int64{
		0, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1_000, 2_500, 5_000, 10_000, 25_000,
		50_000, 100_000, 250_000, 500_000, 1000_000, 2_500_000, 5_000_000, 10_000_000,
	}
	metricHTTPReqCounter       = metrics.LazyLoadCounterVec("api_request_count", []string{"name", "code", "method"})
	metricHTTPReqDuration      = metrics.LazyLoadHistogramVec("api_duration_ms", []string{"name", "code", "method"}, metrics.BucketHTTPReqs)
	metricTxCallVMErrors       = metrics.LazyLoadCounterVec("api_tx_call_vm_errors", []string{"error"})
	metricWebsocketDuration    = metrics.LazyLoadHistogramVec("api_websocket_duration", []string{"name", "code"}, websocketDurations)
	metricActiveWebsocketGauge = metrics.LazyLoadGaugeVec("api_active_websocket_gauge", []string{"name"})
	metricWebsocketCounter     = metrics.LazyLoadCounterVec("api_websocket_counter", []string{"name"})
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
	VMError    string
}

func newCallTxResponseWriter(w http.ResponseWriter) *callTxResponseWriter {
	return &callTxResponseWriter{w, http.StatusOK, ""}
}

func (m *metricsResponseWriter) WriteHeader(code int) {
	m.statusCode = code
	m.ResponseWriter.WriteHeader(code)
}

func (c *callTxResponseWriter) Write(b []byte) (int, error) {
	var resp struct {
		VMError string `json:"VMError"`
	}

	if err := json.Unmarshal(b, &resp); err == nil {
		if resp.VMError != "" {
			c.VMError = resp.VMError
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
			subscription = false
		)

		if rt != nil && rt.GetName() != "" {
			enabled = true
			name = rt.GetName()

			if name == "transactions_call_tx" {
				ctxWriter := newCallTxResponseWriter(w)
				next.ServeHTTP(ctxWriter, r)

				// Record VM error if present
				if ctxWriter.VMError != "" {
					metricTxCallVMErrors().AddWithLabel(1, map[string]string{
						"error": ctxWriter.VMError,
					})
				}
				return
			}

			// Handle subscriptions
			if strings.HasPrefix(name, "subscriptions") {
				subscription = true
				name = "WS " + r.URL.Path
			}
		}

		now := time.Now()
		mrw := newMetricsResponseWriter(w)
		if subscription {
			metricActiveWebsocketGauge().AddWithLabel(1, map[string]string{"name": name})
			metricWebsocketCounter().AddWithLabel(1, map[string]string{"name": name})
		}

		next.ServeHTTP(mrw, r)

		if subscription {
			metricActiveWebsocketGauge().AddWithLabel(-1, map[string]string{"name": name})
			// record websocket duration in seconds, not MS
			metricWebsocketDuration().ObserveWithLabels(time.Since(now).Milliseconds()/1000, map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode)})
		} else if enabled {
			metricHTTPReqCounter().AddWithLabel(1, map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
			metricHTTPReqDuration().ObserveWithLabels(time.Since(now).Milliseconds(), map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
		}
	})
}
