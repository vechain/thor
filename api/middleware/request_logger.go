// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/log"
)

// RequestLoggerMiddleware returns a middleware to ensure requests are syphoned into the writer
func RequestLoggerMiddleware(
	logger log.Logger,
	enabled *atomic.Bool,
	slowQueriesThreshold time.Duration,
	log5xxErrors bool,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If api logging is not enabled and slow queries threshold is set to 0, api logs won't be recorded
			slowQueriesEnabled := slowQueriesThreshold > time.Duration(0) &&
				!strings.HasPrefix(r.URL.Path, "/subscriptions") // disable for all websockets
			if !enabled.Load() && !slowQueriesEnabled && !log5xxErrors {
				next.ServeHTTP(w, r)
				return
			}
			// Read and log the body (note: this can only be done once)
			// Ensure you don't disrupt the request body for handlers that need to read it
			var bodyBytes []byte
			var err error
			if r.Body != nil {
				bodyBytes, err = io.ReadAll(r.Body)
				if err != nil {
					logger.Warn("unexpected body read error", "err", err)
					return // don't pass bad request to the next handler
				}
				r.Body = io.NopCloser(io.Reader(bytes.NewReader(bodyBytes)))
			}

			captor := newStatusCodeCaptor(w)

			start := time.Now()
			next.ServeHTTP(captor, r)
			duration := time.Since(start)

			message := ""
			if enabled.Load() {
				message = "API Request"
			}
			if slowQueriesEnabled && duration > slowQueriesThreshold {
				message = "Slow API Request"
			}
			if log5xxErrors && captor.statusCode >= 500 {
				message = "5xx API Request"
			}

			if message != "" {
				logger.Info(message,
					"DurationMs", duration.Milliseconds(),
					"Timestamp", time.Now().Unix(),
					"URI", r.URL.String(),
					"Method", r.Method,
					"Body", string(bodyBytes),
					"StatusCode", captor.statusCode,
				)
			}
		})
	}
}
