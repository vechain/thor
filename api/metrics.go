// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bufio"
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
	metricHTTPReqCounter    = metrics.LazyLoadCounterVec("api_request_count", []string{"name", "code", "method"})
	metricHTTPReqDuration   = metrics.LazyLoadHistogramVec("api_duration_ms", []string{"name", "code", "method"}, metrics.BucketHTTPReqs)
	metricWebsocketDuration = metrics.LazyLoadHistogramVec("api_websocket_duration", []string{"name", "code"}, metrics.BucketHTTPReqs)
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

func (m *metricsResponseWriter) WriteHeader(code int) {
	m.statusCode = code
	m.ResponseWriter.WriteHeader(code)
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

		// all named route will be recorded
		if rt != nil && rt.GetName() != "" {
			enabled = true
			name = rt.GetName()
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
			metricWebsocketDuration().ObserveWithLabels(time.Since(now).Milliseconds() / 1000, map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode)})
		} else if enabled {
			metricHTTPReqCounter().AddWithLabel(1, map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
			metricHTTPReqDuration().ObserveWithLabels(time.Since(now).Milliseconds(), map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
		}
	})
}
