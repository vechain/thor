// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/cmd/thor/node"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

var ts *httptest.Server

func masterNode() (*node.Master, string) {
	privKey, _ := crypto.HexToECDSA("99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36")
	masterNode := &node.Master{
		PrivateKey: privKey,
	}
	return masterNode, crypto.PubkeyToAddress(privKey.PublicKey).Hex()
}

func TestHealth(t *testing.T) {
	initAPIServer(t)

	_, masterAddr := masterNode()
	var healthStatus api.HealthStatus
	respBody, statusCode := httpGet(t, ts.URL+"/health")
	require.NoError(t, json.Unmarshal(respBody, &healthStatus))
	assert.False(t, healthStatus.Healthy)
	assert.Equal(t, http.StatusServiceUnavailable, statusCode)
	assert.Equal(t, healthStatus.NodeMaster, &masterAddr)
	assert.Nil(t, healthStatus.Beneficiary)
}

func initAPIServer(t *testing.T) {
	thorChain, err := testchain.NewDefault()
	thorChain.Genesis()
	require.NoError(t, err)
	masterNode, _ := masterNode()
	router := mux.NewRouter()
	NewAPI(
		New(thorChain.Repo(), comm.New(thorChain.Repo(), txpool.New(thorChain.Repo(), nil, txpool.Options{}, &thor.NoFork), nil)), masterNode,
	).Mount(router, "/health")

	ts = httptest.NewServer(router)
}

func httpGet(t *testing.T, url string) ([]byte, int) {
	res, err := http.Get(url) //#nosec G107
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	r, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}
