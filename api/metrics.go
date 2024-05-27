// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/metrics"
)

var (
	metricHttpReqCounter  = metrics.LazyLoadCounterVec("api_request_count", []string{"name", "code", "method"})
	metricHttpReqDuration = metrics.LazyLoadHistogramVec("api_duration_ms", []string{"name", "code", "method"}, metrics.BucketHTTPReqs)
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

// metricsMiddleware is a middleware that records metrics for each request.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt := mux.CurrentRoute(r)

		var (
			enabled = false
			name    = ""
		)

		// all named route will be recorded
		if rt != nil && rt.GetName() != "" {
			enabled = true
			name = rt.GetName()
		}

		now := time.Now()
		mrw := newMetricsResponseWriter(w)

		next.ServeHTTP(mrw, r)

		if enabled {
			metricHttpReqCounter().AddWithLabel(1, map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
			metricHttpReqDuration().ObserveWithLabels(time.Since(now).Milliseconds(), map[string]string{"name": name, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
		}
	})
}
