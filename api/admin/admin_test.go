// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/admin"
	healthAPI "github.com/vechain/thor/v2/api/admin/health"
	apinode "github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/cmd/thor/node"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// TestAdminToggleAffectsNodeAPI is the e2e contract test: flipping
// /admin/features/txpool-api via the admin server must immediately gate
// /node/txpool on the business API server, via the shared atomic.Bool.
func TestAdminToggleAffectsNodeAPI(t *testing.T) {
	chain, err := testchain.NewDefault()
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{
		Limit: 100, LimitPerAccount: 16, MaxLifetime: time.Minute,
	}, &thor.NoFork)
	defer pool.Close()

	enableTxPool := &atomic.Bool{}
	enableTxPool.Store(true)
	txpoolGate := admin.NewGate("txpool-api", enableTxPool)
	apiLogsGate := admin.NewGate("apilogs", &atomic.Bool{})
	pprofGate := admin.NewGate("pprof", &atomic.Bool{})

	// Admin server
	privKey, _ := crypto.HexToECDSA("99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36")
	master := &node.Master{PrivateKey: privKey}
	adminHandler := admin.NewHTTPHandler(
		&slog.LevelVar{},
		healthAPI.New(chain.Repo(), comm.New(chain.Repo(), pool)),
		apiLogsGate, txpoolGate, pprofGate,
		master,
	)
	adminTS := httptest.NewServer(adminHandler)
	defer adminTS.Close()

	// Business API server, sharing enableTxPool with the admin gate
	nodeRouter := mux.NewRouter()
	apinode.New(comm.New(chain.Repo(), pool), pool, enableTxPool).Mount(nodeRouter, "/node")
	nodeTS := httptest.NewServer(nodeRouter)
	defer nodeTS.Close()

	// Sanity: initially enabled
	require.Equal(t, http.StatusOK, getStatus(t, nodeTS.URL+"/node/txpool"))

	// Toggle off via admin
	body, _ := json.Marshal(api.ToggleStatus{Enabled: false})
	resp, err := http.Post(adminTS.URL+"/admin/features/txpool-api", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Business endpoint now 503
	assert.Equal(t, http.StatusServiceUnavailable, getStatus(t, nodeTS.URL+"/node/txpool"))

	// Toggle back on via admin
	body, _ = json.Marshal(api.ToggleStatus{Enabled: true})
	resp, err = http.Post(adminTS.URL+"/admin/features/txpool-api", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, http.StatusOK, getStatus(t, nodeTS.URL+"/node/txpool"))
}

func getStatus(t *testing.T, url string) int {
	t.Helper()
	res, err := http.Get(url) //#nosec G107
	require.NoError(t, err)
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode
}
