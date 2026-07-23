// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDummyServer(t *testing.T) *Server {
	srv := New()
	require.NoError(t, srv.RegisterName("dummy", dummy{}))
	return srv
}

func TestHandleMsg(t *testing.T) {
	srv := newDummyServer(t)
	ctx := context.Background()

	// success
	resp := srv.handleMsg(ctx, &jsonrpcMessage{
		Version: "2.0", ID: json.RawMessage("1"),
		Method: "dummy_echo", Params: json.RawMessage(`["hi"]`),
	})
	assert.Nil(t, resp.Error)
	assert.Equal(t, "hi", resp.Result)

	// wrong version -> -32600
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "1.0", ID: json.RawMessage("2"), Method: "dummy_echo"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeInvalidRequest, resp.Error.Code)

	// unknown method -> -32601
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "2.0", ID: json.RawMessage("3"), Method: "dummy_nope"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeMethodNotFound, resp.Error.Code)

	// bad params -> -32602
	resp = srv.handleMsg(ctx, &jsonrpcMessage{
		Version: "2.0", ID: json.RawMessage("4"), Method: "dummy_echo",
		Params: json.RawMessage(`[1,2]`),
	})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeInvalidParams, resp.Error.Code)

	// panic -> -32603
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "2.0", ID: json.RawMessage("5"), Method: "dummy_boom"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeInternal, resp.Error.Code)

	// notification (no id) -> no reply
	resp = srv.handleMsg(ctx, &jsonrpcMessage{
		Version: "2.0", Method: "dummy_echo", Params: json.RawMessage(`["hi"]`),
	})
	assert.Nil(t, resp)
}

func TestServeHTTP(t *testing.T) {
	srv := newDummyServer(t)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"dummy_echo","params":["hi"]}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"hi"}`, w.Body.String())
}
