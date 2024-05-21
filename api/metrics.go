// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vechain/thor/v2/metrics"
)

var (
	metricHttpReqCounter  = metrics.CounterVec("api_request_count", []string{"path", "code", "method"})
	metricHttpReqDuration = metrics.HistogramVec("api_duration_ms", []string{"path", "code", "method"}, metrics.BucketHTTPReqs)
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

// metricsHandler is a middleware that records metrics for each request.
func metricsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()

		mrw := newMetricsResponseWriter(w)
		h.ServeHTTP(mrw, r)

		url := strings.ReplaceAll(strings.TrimLeft(r.URL.Path, "/"), "/", "_") // ensure no unexpected slashes
		metricHttpReqCounter.AddWithLabel(1, map[string]string{"path": url, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
		metricHttpReqDuration.ObserveWithLabels(time.Since(now).Milliseconds(), map[string]string{"path": url, "code": strconv.Itoa(mrw.statusCode), "method": r.Method})
	})
}
