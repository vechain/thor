// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/log"
)

// newTestServerWithLog returns a Server with EnableReqLogger set to true and
// a slog.Logger capturing output in the returned *bytes.Buffer. The previous
// slog default is restored by t.Cleanup.
//
// The server has the same dispatch table as newTestServer plus an
// "eth_blockNumber" stub that returns a fixed hex string (no repo needed).
func newTestServerWithLog(t *testing.T) (*Server, *bytes.Buffer) {
	t.Helper()

	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	prev := log.Root()
	log.SetDefault(log.NewLogger(h))
	t.Cleanup(func() { log.SetDefault(prev) })

	flag := &atomic.Bool{}
	flag.Store(true)

	s := &Server{
		cfg: Config{
			BodyLimit:       4096,
			EnableReqLogger: flag,
		},
		dispatch: map[string]handlerFunc{},
	}

	// Shared handlers from the standard test suite.
	s.dispatch["eth_echo"] = func(_ context.Context, _ *Server, params json.RawMessage) (any, *RPCError) {
		if len(params) == 0 {
			return nil, nil
		}
		var args []any
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, InvalidParams(err.Error())
		}
		if len(args) == 0 {
			return nil, nil
		}
		return args[0], nil
	}
	s.dispatch["eth_boom"] = func(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
		return nil, ReasonError(ReasonExecutionReverted, "revert reason here")
	}

	// Stub for eth_blockNumber — returns a fixed hex string without needing repo.
	s.dispatch["eth_blockNumber"] = func(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
		return "0x64", nil // block 100
	}

	// Stub for eth_sendRawTransaction — returns a 66-char hex txid.
	s.dispatch["eth_sendRawTransaction"] = func(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
		return "0x" + strings.Repeat("a", 64), nil
	}

	return s, buf
}

// countInfoLines returns the number of lines in buf that contain "level=INFO".
func countInfoLines(buf *bytes.Buffer) int {
	n := 0
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if strings.Contains(line, "level=INFO") {
			n++
		}
	}
	return n
}

// --- TestLogger_AllExits -------------------------------------------------------
// V2: every exit path emits exactly one log line.

func TestLogger_AllExits(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		bodyLimit    int64 // 0 = use default
		wantMethod   string
		wantCode     string
		wantReason   string
		wantNoReason bool
	}{
		{
			name:       "oversized_body",
			body:       strings.Repeat("a", 20),
			bodyLimit:  16,
			wantMethod: "(unknown)",
			wantCode:   "code=-32000",
			wantReason: "reason=oversized_data",
		},
		{
			name:       "batch_reject",
			body:       `[{"jsonrpc":"2.0","method":"eth_echo","id":1}]`,
			wantMethod: "(unknown)",
			wantCode:   "code=-32600",
			wantReason: "reason=invalid_request",
		},
		{
			name:       "parse_error",
			body:       `garbage`,
			wantMethod: "(unknown)",
			wantCode:   "code=-32700",
			wantReason: "reason=parse_error",
		},
		{
			name:       "invalid_version",
			body:       `{"jsonrpc":"1.0","method":"eth_blockNumber","id":1}`,
			wantMethod: "(unknown)",
			wantCode:   "code=-32600",
			wantReason: "reason=invalid_request",
		},
		{
			name:       "empty_method",
			body:       `{"jsonrpc":"2.0","method":"","id":1}`,
			wantMethod: "(unknown)",
			wantCode:   "code=-32600",
			wantReason: "reason=invalid_request",
		},
		{
			name:       "unsupported_method",
			body:       `{"jsonrpc":"2.0","method":"eth_subscribe","id":1}`,
			wantMethod: "method=eth_subscribe",
			wantCode:   "code=-32601",
			wantReason: "reason=method_not_found",
		},
		{
			name:         "success",
			body:         `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`,
			wantMethod:   "method=eth_blockNumber",
			wantCode:     "code=0",
			wantNoReason: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, buf := newTestServerWithLog(t)
			if tc.bodyLimit > 0 {
				s.cfg.BodyLimit = tc.bodyLimit
			}

			req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)

			lines := countInfoLines(buf)
			assert.Equal(t, 1, lines, "expected exactly 1 INFO log line; got %d:\n%s", lines, buf.String())

			out := buf.String()
			assert.Contains(t, out, tc.wantMethod, "method field mismatch")
			assert.Contains(t, out, tc.wantCode, "code field mismatch")

			if tc.wantReason != "" {
				assert.Contains(t, out, tc.wantReason, "reason field mismatch")
			}
			if tc.wantNoReason {
				assert.NotContains(t, out, "reason=", "success path must not have reason field")
			}
		})
	}
}

// --- TestLogger_DisabledWhenNil ------------------------------------------------
// V2 guard: no log emitted when EnableReqLogger is nil.

func TestLogger_DisabledWhenNil(t *testing.T) {
	s, buf := newTestServerWithLog(t)
	s.cfg.EnableReqLogger = nil // simulate disabled

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	lines := countInfoLines(buf)
	assert.Equal(t, 0, lines, "expected 0 log lines when logger is nil; got:\n%s", buf.String())
}

// TestLogger_DisabledWhenFalse: no log emitted when EnableReqLogger.Load() == false.
func TestLogger_DisabledWhenFalse(t *testing.T) {
	s, buf := newTestServerWithLog(t)
	s.cfg.EnableReqLogger.Store(false)

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	lines := countInfoLines(buf)
	assert.Equal(t, 0, lines, "expected 0 log lines when logger disabled; got:\n%s", buf.String())
}

// --- TestLogger_PreviewTruncation ---------------------------------------------

func TestLogger_PreviewTruncation(t *testing.T) {
	s, buf := newTestServerWithLog(t)

	// Build a valid JSON-RPC request with a params array containing a 250-byte string.
	// The whole body will be > 200 bytes → triggers truncation in paramsPreview.
	longParam := strings.Repeat("x", 250)
	body := `{"jsonrpc":"2.0","method":"eth_echo","params":["` + longParam + `"],"id":1}`

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	out := buf.String()
	assert.Contains(t, out, "...(truncated)", "expected truncation suffix in params_preview")
	assert.Equal(t, 1, countInfoLines(buf), "expected exactly 1 log line")
}

// --- TestLogger_ResultPreview -------------------------------------------------

func TestLogger_ResultPreview(t *testing.T) {
	t.Run("eth_blockNumber_preview", func(t *testing.T) {
		s, buf := newTestServerWithLog(t)
		req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		out := buf.String()
		assert.Contains(t, out, "result_preview=0x64", "eth_blockNumber should have result_preview")
	})

	t.Run("eth_sendRawTransaction_preview_shortened", func(t *testing.T) {
		s, buf := newTestServerWithLog(t)
		req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0xdeadbeef"],"id":1}`))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		// The stub returns "0x" + 64 'a' chars = 66-char hex → shortHex trims it.
		out := buf.String()
		assert.Contains(t, out, "result_preview=", "eth_sendRawTransaction must have result_preview")
		assert.Contains(t, out, "...", "66-char hex must be shortened via shortHex")
	})

	t.Run("eth_echo_no_preview", func(t *testing.T) {
		s, buf := newTestServerWithLog(t)
		req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","method":"eth_echo","params":["hello"],"id":1}`))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		out := buf.String()
		assert.NotContains(t, out, "result_preview=", "eth_echo must not have result_preview")
	})
}

// --- TestLogger_AlwaysHasMethod -----------------------------------------------
// V2': `method` field present in every log line regardless of exit path.

func TestLogger_AlwaysHasMethod(t *testing.T) {
	scenarios := []struct {
		name      string
		body      string
		bodyLimit int64
	}{
		{"oversized_body", strings.Repeat("a", 20), 16},
		{"batch_reject", `[{"jsonrpc":"2.0","method":"eth_echo","id":1}]`, 0},
		{"parse_error", `garbage`, 0},
		{"invalid_version", `{"jsonrpc":"1.0","method":"eth_blockNumber","id":1}`, 0},
		{"empty_method", `{"jsonrpc":"2.0","method":"","id":1}`, 0},
		{"unsupported_method", `{"jsonrpc":"2.0","method":"eth_subscribe","id":1}`, 0},
		{"success", `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`, 0},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			s, buf := newTestServerWithLog(t)
			if sc.bodyLimit > 0 {
				s.cfg.BodyLimit = sc.bodyLimit
			}

			req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(sc.body))
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)

			out := buf.String()
			assert.Contains(t, out, "method=", "V2': method field must always be present; scenario=%s log=%s", sc.name, out)
		})
	}
}

// --- TestResultSummary (unit test of helper) -----------------------------------

func TestResultSummary(t *testing.T) {
	// shortHex boundary cases.
	assert.Equal(t, "0xabc", shortHex("0xabc"), "short hex unchanged")
	assert.Equal(t, "0xabc1234567890123456789012345678901234567890123456789012345678abcd"[:7]+"..."+"abcd",
		shortHex("0xabc1234567890123456789012345678901234567890123456789012345678abcd"))

	// resultSummary for sendRawTransaction.
	longHex := "0x" + strings.Repeat("a", 64) // 66 chars
	preview := resultSummary("eth_sendRawTransaction", longHex)
	require.NotEmpty(t, preview)
	assert.True(t, strings.HasSuffix(preview, longHex[len(longHex)-4:]), "suffix preserved")
	assert.True(t, strings.HasPrefix(preview, longHex[:7]), "prefix preserved")
	assert.Contains(t, preview, "...", "ellipsis present")

	// Short txid: no ellipsis.
	shortTxid := "0xdeadbeef"
	assert.Equal(t, shortTxid, resultSummary("eth_sendRawTransaction", shortTxid))

	// blockNumber returns value as-is.
	assert.Equal(t, "0x64", resultSummary("eth_blockNumber", "0x64"))

	// No preview for other methods.
	assert.Equal(t, "", resultSummary("eth_getLogs", []string{}))
	assert.Equal(t, "", resultSummary("eth_call", "0xresult"))
}

// --- TestParamsPreview (unit test of helper) -----------------------------------

func TestParamsPreview(t *testing.T) {
	short := []byte(`["hello","world"]`)
	assert.Equal(t, string(short), paramsPreview(short))

	long := []byte(strings.Repeat("x", 250))
	preview := paramsPreview(long)
	assert.True(t, strings.HasSuffix(preview, "...(truncated)"))
	assert.Equal(t, paramsPreviewLimit+len("...(truncated)"), len(preview))

	exact := []byte(strings.Repeat("x", paramsPreviewLimit))
	assert.Equal(t, string(exact), paramsPreview(exact), "exactly at limit: no truncation")
}

// --- TestLogger_ResultSize ----------------------------------------------------

func TestLogger_ResultSize(t *testing.T) {
	s, buf := newTestServerWithLog(t)
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	out := buf.String()
	assert.Contains(t, out, "result_size=", "success path must include result_size field")
	assert.NotContains(t, out, "result_size=0", "result_size should be > 0 for non-nil result")
}
