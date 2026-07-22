// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
)

func newHTTPServer(t *testing.T) *httptest.Server {
	tc, err := testchain.NewDefault()
	require.NoError(t, err)
	router := mux.NewRouter()
	New(tc.Repo(), tc.Stater(), tc.Engine(), tc.GetForkConfig()).Mount(router, "/rpc")
	return httptest.NewServer(router)
}

func post(t *testing.T, url, body string) string {
	resp, err := http.Post(url, "application/json", strings.NewReader(body)) //#nosec G107
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(out)
}

func TestHTTPSingleAndBatch(t *testing.T) {
	ts := newHTTPServer(t)
	defer ts.Close()
	url := ts.URL + "/rpc"

	// single: blockNumber on genesis-only chain
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"0x0"}`,
		post(t, url, `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`))

	// getBalance of the zero address => 0
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":2,"result":"0x0"}`,
		post(t, url, `{"jsonrpc":"2.0","id":2,"method":"eth_getBalance","params":["0x0000000000000000000000000000000000000000"]}`))

	// batch of two
	assert.JSONEq(t, `[{"jsonrpc":"2.0","id":1,"result":"0x0"},{"jsonrpc":"2.0","id":2,"result":"thor"}]`,
		post(t, url, `[{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"},{"jsonrpc":"2.0","id":2,"method":"web3_clientVersion"}]`))

	// unknown method -> -32601
	resp := post(t, url, `{"jsonrpc":"2.0","id":9,"method":"eth_nope"}`)
	assert.Contains(t, resp, "-32601")
}
