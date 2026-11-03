// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package middleware

import (
	"bufio"
	"context"
	"io"
	"net"
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

// HandleAPITimeout deadlines the request context, bounding handler work that
// honours it (state reads, EVM calls, logdb queries).
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

// writeChunkSize is the most a single deadline-bounded write hands to the connection.
const writeChunkSize = 32 * 1024

// HandleWriteDeadline bounds how long a response write may stall: every chunk of up
// to writeChunkSize bytes must reach the connection within timeout. A client that
// stops reading is cut off, while a slow one still receives a large response.
//
// The deadline is armed by the first write, so time spent before the response
// starts is left to HandleAPITimeout. It is armed once more when the handler
// returns, because net/http sends whatever is still buffered (and the whole of a
// response that only set a status) after that, in finishRequest. net/http clears
// it once the response is finished, before the connection serves its next request.
//
// It must wrap the server's whole handler, not be a router middleware: mux runs
// router middlewares only on matched routes, so its 404/405 replies and CORS
// preflights answered outside the router would go out unbounded.
//
// Every request is wrapped, websocket upgrades included: request headers are
// client-controlled, so they cannot opt a response out. An upgrade takes the
// connection through Hijack, after which its writes are outside this middleware's
// reach and the final arm is skipped: net/http's SetWriteDeadline does not check
// for a hijack and would set the deadline on the upgraded connection.
func HandleWriteDeadline(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dw := &deadlineWriter{
				ResponseWriter: w,
				rc:             http.NewResponseController(w),
				timeout:        timeout,
			}
			next.ServeHTTP(dw, r)
			if !dw.hijacked {
				dw.extend()
			}
		})
	}
}

type deadlineWriter struct {
	http.ResponseWriter
	rc       *http.ResponseController
	timeout  time.Duration
	hijacked bool // set by Hijack; read after the handler returns, on the same goroutine
}

// extend pushes the write deadline out by timeout. Writers without deadline
// support (such as httptest.ResponseRecorder) are not connections and cannot
// stall, so the error is ignored.
func (d *deadlineWriter) extend() {
	_ = d.rc.SetWriteDeadline(time.Now().Add(d.timeout))
}

func (d *deadlineWriter) Write(b []byte) (int, error) {
	var n int
	for len(b) > 0 {
		d.extend()
		m, err := d.ResponseWriter.Write(b[:min(len(b), writeChunkSize)])
		n += m
		if err != nil {
			return n, err
		}
		b = b[m:]
	}
	return n, nil
}

func (d *deadlineWriter) Flush() {
	d.extend()
	_ = d.rc.Flush()
}

func (d *deadlineWriter) Unwrap() http.ResponseWriter {
	return d.ResponseWriter
}

// Hijack is forwarded for websocket upgrades: gorilla's Upgrader type-asserts
// http.Hijacker on the writer it is given rather than unwrapping it.
func (d *deadlineWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := d.rc.Hijack()
	if err == nil {
		d.hijacked = true
	}
	return conn, rw, err
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
						println("PANIC in HTTP handler:", string(debug.Stack()))
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
