// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package middleware

import (
	"context"
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/vechain/thor/v2/api/doc"
	"github.com/vechain/thor/v2/thor"
)

// middleware to verify 'x-genesis-id' header in request, and set to response headers.
func HandleXGenesisID(genesisID thor.Bytes32) func(http.Handler) http.Handler {
	const headerKey = "x-genesis-id"
	expectedID := genesisID.String()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actualID := r.Header.Get(headerKey)
			if actualID == "" {
				actualID = r.URL.Query().Get(headerKey)
			}
			w.Header().Set(headerKey, expectedID)
			if actualID != "" && actualID != expectedID {
				io.Copy(io.Discard, r.Body)
				http.Error(w, "genesis id mismatch", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// middleware to set 'x-thorest-ver' to response headers.
func HandleXThorestVersion(next http.Handler) http.Handler {
	const headerKey = "x-thorest-ver"
	ver := doc.Version()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerKey, ver)
		next.ServeHTTP(w, r)
	})
}

// middleware for http request timeout.
func HandleAPITimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// middleware to limit request body size.
func HandleRequestBodyLimit(maxBodySize int64) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
			next.ServeHTTP(w, r)
		})
	}
}

// HandlePanics is a middleware to recover panics in HTTP handlers.
// If logEnabled is true, the stack trace will be printed to the standard output.
func HandlePanics(logEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					status := http.StatusInternalServerError
					text := http.StatusText(status)
					http.Error(w, text, status)
					if logEnabled {
						println(string(debug.Stack()))
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
