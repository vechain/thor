// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/vechain/thor/v2/log"
)

// RequestLoggerHandler returns a http handler to ensure requests are syphoned into the writer
func RequestLoggerHandler(handler http.Handler, logger log.Logger) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// Read and logger the body (note: this can only be done once)
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

		logger.Info("API Request",
			"timestamp", time.Now().Unix(),
			"URI", r.URL.String(),
			"Method", r.Method,
			"Body", string(bodyBytes),
		)

		// call the original http.Handler we're wrapping
		handler.ServeHTTP(w, r)
	}

	// http.HandlerFunc wraps a function so that it
	// implements http.Handler interface
	return http.HandlerFunc(fn)
}
