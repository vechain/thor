// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestServer returns a Server with no Thor dependencies wired and a single
// echo handler installed at "eth_echo". Handler tests in later tasks cover the
// real dependencies; this suite focuses on the dispatch pipeline.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		cfg:      Config{BodyLimit: 4096},
		dispatch: map[string]handlerFunc{},
	}
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
	s.dispatch["eth_null"] = func(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
		return nil, nil
	}
	return s
}

// doRPC posts body to s and returns the full response envelope plus HTTP status.
func doRPC(t *testing.T, s *Server, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "raw: %s", rec.Body.String())
	return rec.Code, env
}

func TestServer_MethodNotPOST_Returns405(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "POST", rec.Header().Get("Allow"))
}

func TestServer_ParseError_MalformedJSON(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `{not json`)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeParseError, e.Code)
	// ID is null when the request couldn't be decoded.
	assert.Equal(t, json.RawMessage("null"), env["id"])
}

func TestServer_BatchRequestsRejected(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `[{"jsonrpc":"2.0","method":"eth_echo","id":1}]`)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeInvalidRequest, e.Code)
	assert.Contains(t, e.Message, "batch requests not supported")
}

func TestServer_WrongJSONRPCVersion(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `{"jsonrpc":"1.0","method":"eth_echo","id":7}`)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeInvalidRequest, e.Code)
	// ID echoes through.
	assert.Equal(t, json.RawMessage("7"), env["id"])
}

func TestServer_MissingMethod(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `{"jsonrpc":"2.0","id":1}`)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeInvalidRequest, e.Code)
	assert.Contains(t, e.Message, "method required")
}

func TestServer_UnknownMethod(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `{"jsonrpc":"2.0","method":"eth_notAThing","id":"abc"}`)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeMethodNotFound, e.Code)
	assert.Equal(t, json.RawMessage(`"abc"`), env["id"], "string ID round-trips")
}

func TestServer_Success_IDEcho(t *testing.T) {
	s := newTestServer(t)

	cases := []struct {
		name    string
		id      string
		wantRaw string
	}{
		{"numeric-id", `42`, `42`},
		{"string-id", `"abc-123"`, `"abc-123"`},
		{"null-id", `null`, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, env := doRPC(t, s, `{"jsonrpc":"2.0","method":"eth_echo","params":["ping"],"id":`+tc.id+`}`)
			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, json.RawMessage(tc.wantRaw), env["id"])
			_, hasErr := env["error"]
			assert.False(t, hasErr, "success envelope must not carry error field")
			assert.Equal(t, json.RawMessage(`"ping"`), env["result"])
		})
	}
}

func TestServer_NullSuccess_EmitsResultNull(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `{"jsonrpc":"2.0","method":"eth_null","id":1}`)
	assert.Equal(t, http.StatusOK, code)
	// `result: null` must appear (distinguishes "not found" from "error").
	assert.Equal(t, json.RawMessage(`null`), env["result"])
	_, hasErr := env["error"]
	assert.False(t, hasErr, "null-success envelope must not carry error field")
}

func TestServer_ReasonError_EmitsData(t *testing.T) {
	s := newTestServer(t)
	code, env := doRPC(t, s, `{"jsonrpc":"2.0","method":"eth_boom","id":1}`)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeServerError, e.Code)
	data, ok := e.Data.(map[string]any)
	require.True(t, ok, "data must be an object; got %T", e.Data)
	assert.Equal(t, ReasonExecutionReverted, data["reason"])
}

func TestServer_OversizedBody_Reason(t *testing.T) {
	s := newTestServer(t)
	s.cfg.BodyLimit = 10 // tiny

	big := `{"jsonrpc":"2.0","method":"eth_echo","params":["` + strings.Repeat("x", 100) + `"],"id":1}`
	code, env := doRPC(t, s, big)
	assert.Equal(t, http.StatusOK, code)
	var e RPCError
	require.NoError(t, json.Unmarshal(env["error"], &e))
	assert.Equal(t, CodeServerError, e.Code)
	data, _ := e.Data.(map[string]any)
	assert.Equal(t, ReasonOversizedData, data["reason"])
}

func TestFirstNonSpace(t *testing.T) {
	assert.Equal(t, byte('{'), firstNonSpace([]byte("  \t\n{x}")))
	assert.Equal(t, byte('['), firstNonSpace([]byte("\r\n[1]")))
	assert.Equal(t, byte(0), firstNonSpace([]byte("   \t\n")))
	assert.Equal(t, byte(0), firstNonSpace(nil))
}

func TestRegister_DuplicatePanics(t *testing.T) {
	// Isolate the global map by saving and restoring.
	saved := maps.Clone(globalHandlers)
	defer func() {
		globalHandlers = saved
	}()

	globalHandlers = map[string]handlerFunc{}
	register("eth_duplicateTest", func(context.Context, *Server, json.RawMessage) (any, *RPCError) { return nil, nil })
	assert.PanicsWithValue(t, "rpc: duplicate handler registration for eth_duplicateTest", func() {
		register("eth_duplicateTest", func(context.Context, *Server, json.RawMessage) (any, *RPCError) { return nil, nil })
	})
}

// Compile-time / readability sanity: a handlerFunc is invocable with the right
// argument shape. This is trivial but prevents silent signature drift when
// later tasks refactor the dispatch contract.
var _ handlerFunc = func(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) { return nil, nil }

// Silence lint by referencing io / bytes in a trivial way if they become
// unused later.
func TestServer_IO_Minimal(t *testing.T) {
	// Make sure Content-Type is set on success responses.
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","method":"eth_echo","params":[123],"id":1}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	b, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"result":123`)
}
