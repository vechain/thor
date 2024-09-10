// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package node_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/txpool"

	thornode "github.com/vechain/thor/v2/node"
)

var ts *httptest.Server

func TestNode(t *testing.T) {
	initCommServer(t)
	res := httpGet(t, ts.URL+"/node/network/peers")
	var peersStats map[string]string
	if err := json.Unmarshal(res, &peersStats); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 0, len(peersStats), "count should be zero")
}

func initCommServer(t *testing.T) {
	thorChain, err := thornode.NewIntegrationTestChain()
	require.NoError(t, err)

	comm := comm.New(thorChain.Repo(), txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}))

	thorNode, err := new(thornode.Builder).
		WithChain(thorChain).
		WithAPIs(node.New(comm)).
		Build()
	require.NoError(t, err)

	ts = httptest.NewServer(thorNode.Router())
}

func httpGet(t *testing.T, url string) []byte {
	res, err := http.Get(url) // nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
