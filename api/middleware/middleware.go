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
// The middleware arms the deadline at three points:
//   - Before the handler, for the data that net/http writes first, for example
//     100 Continue.
//   - Before each write. Thus, the time before the handler writes is not part
//     of this deadline. HandleAPITimeout applies to that time.
//   - After the handler, for the data that net/http sends in finishRequest,
//     for example a response that has only a status.
//
// net/http removes the deadline before the next request on the connection.
//
// This middleware must wrap the full server handler, not only the router. mux
// does not use router middlewares for its 404 and 405 replies, and the router
// does not get CORS preflight requests.
//
// A request header cannot remove a response from the deadline, also for a
// websocket upgrade. Hijack removes the deadlines of the connection. After a
// hijack, the middleware does not do the last arm, because SetWriteDeadline
// does not examine if the connection is hijacked.
func HandleWriteDeadline(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dw := &deadlineWriter{
				ResponseWriter: w,
				rc:             http.NewResponseController(w),
				timeout:        timeout,
			}
			dw.extend()
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
	hijacked bool // Hijack writes it. The same goroutine reads it after the handler returns.
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
