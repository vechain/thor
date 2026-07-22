// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
)

func newTestBackend(t *testing.T) *backend {
	tc, err := testchain.NewDefault()
	require.NoError(t, err)
	return &backend{
		repo:       tc.Repo(),
		stater:     tc.Stater(),
		bft:        tc.Engine(),
		forkConfig: tc.GetForkConfig(),
	}
}

func newMethodServer(t *testing.T) *Server {
	b := newTestBackend(t)
	srv := NewServer()
	require.NoError(t, srv.RegisterName("eth", &ethAPI{b: b}))
	require.NoError(t, srv.RegisterName("net", &netAPI{b: b}))
	require.NoError(t, srv.RegisterName("web3", &web3API{}))
	return srv
}

func dispatchJSON(t *testing.T, srv *Server, method string) string {
	resp := srv.handleMsg(context.Background(),
		&jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"), Method: method})
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	return string(out)
}

func TestExampleMethods(t *testing.T) {
	srv := newMethodServer(t)

	// genesis-only chain: best block number is 0 -> "0x0"
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"0x0"}`, dispatchJSON(t, srv, "eth_blockNumber"))

	// web3_clientVersion is a fixed string
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"thor"}`, dispatchJSON(t, srv, "web3_clientVersion"))

	// chainId and net_version are deterministic per genesis but value not asserted; must not error
	resp := srv.handleMsg(context.Background(),
		&jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"), Method: "eth_chainId"})
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	resp = srv.handleMsg(context.Background(),
		&jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"), Method: "net_version"})
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)
}
