// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bytes"
	"crypto/rand"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

func init() {
	metrics.InitializePrometheusMetrics()
}

func TestMetricsMiddleware(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	// inject some invalid data to db
	data := thorChain.Database().NewStore("chain.data")
	var blkID thor.Bytes32
	rand.Read(blkID[:])
	data.Put(blkID[:], []byte("invalid data"))

	// get summary should fail since the block data is not rlp encoded
	_, err = thorChain.Repo().GetBlockSummary(blkID)
	assert.NotNil(t, err)

	router := mux.NewRouter()
	acc := accounts.New(thorChain.Repo(), thorChain.Stater(), math.MaxUint64, thor.NoFork, thorChain.Engine())
	acc.Mount(router, "/accounts")
	router.PathPrefix("/metrics").Handler(metrics.HTTPHandler())
	router.Use(metricsMiddleware)
	ts := httptest.NewServer(router)

	httpGet(t, ts.URL+"/accounts/0x")
	httpGet(t, ts.URL+"/accounts/"+thor.Address{}.String())

	_, code := httpGet(t, ts.URL+"/accounts/"+thor.Address{}.String()+"?revision="+blkID.String())
	assert.Equal(t, 500, code)

	body, _ := httpGet(t, ts.URL+"/metrics")
	parser := expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	assert.Nil(t, err)

	m := metrics["thor_metrics_api_request_count"].GetMetric()
	assert.Equal(t, 3, len(m), "should be 3 metric entries")
	assert.Equal(t, float64(1), m[0].GetCounter().GetValue())
	assert.Equal(t, float64(1), m[1].GetCounter().GetValue())

	labels := m[0].GetLabel()
	assert.Equal(t, 3, len(labels))
	assert.Equal(t, "code", labels[0].GetName())
	assert.Equal(t, "200", labels[0].GetValue())
	assert.Equal(t, "method", labels[1].GetName())
	assert.Equal(t, "GET", labels[1].GetValue())
	assert.Equal(t, "name", labels[2].GetName())
	assert.Equal(t, "accounts_get_account", labels[2].GetValue())

	labels = m[1].GetLabel()
	assert.Equal(t, 3, len(labels))
	assert.Equal(t, "code", labels[0].GetName())
	assert.Equal(t, "400", labels[0].GetValue())
	assert.Equal(t, "method", labels[1].GetName())
	assert.Equal(t, "GET", labels[1].GetValue())
	assert.Equal(t, "name", labels[2].GetName())
	assert.Equal(t, "accounts_get_account", labels[2].GetValue())

	labels = m[2].GetLabel()
	assert.Equal(t, 3, len(labels))
	assert.Equal(t, "code", labels[0].GetName())
	assert.Equal(t, "500", labels[0].GetValue())
	assert.Equal(t, "method", labels[1].GetName())
	assert.Equal(t, "GET", labels[1].GetValue())
	assert.Equal(t, "name", labels[2].GetName())
	assert.Equal(t, "accounts_get_account", labels[2].GetValue())
}

func TestWebsocketMetrics(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	router := mux.NewRouter()
	sub := subscriptions.New(thorChain.Repo(), []string{"*"}, 10, txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{}))
	sub.Mount(router, "/subscriptions")
	router.PathPrefix("/metrics").Handler(metrics.HTTPHandler())
	router.Use(metricsMiddleware)
	ts := httptest.NewServer(router)

	// initiate 1 beat subscription, active websocket should be 1
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/beat"}
	conn1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.Nil(t, err)
	defer conn1.Close()

	body, _ := httpGet(t, ts.URL+"/metrics")
	parser := expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	assert.Nil(t, err)

	m := metrics["thor_metrics_api_active_websocket_count"].GetMetric()
	assert.Equal(t, 1, len(m), "should be 1 metric entries")
	assert.Equal(t, float64(1), m[0].GetGauge().GetValue())

	labels := m[0].GetLabel()
	assert.Equal(t, "subject", labels[0].GetName())
	assert.Equal(t, "beat", labels[0].GetValue())

	// initiate 1 beat subscription, active websocket should be 2
	conn2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.Nil(t, err)
	defer conn2.Close()

	body, _ = httpGet(t, ts.URL+"/metrics")
	metrics, err = parser.TextToMetricFamilies(bytes.NewReader(body))
	assert.Nil(t, err)

	m = metrics["thor_metrics_api_active_websocket_count"].GetMetric()
	assert.Equal(t, 1, len(m), "should be 1 metric entries")
	assert.Equal(t, float64(2), m[0].GetGauge().GetValue())

	// initiate 1 block subscription, active websocket should be 3
	u = url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/block"}
	conn3, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.Nil(t, err)
	defer conn3.Close()

	body, _ = httpGet(t, ts.URL+"/metrics")
	metrics, err = parser.TextToMetricFamilies(bytes.NewReader(body))
	assert.Nil(t, err)

	m = metrics["thor_metrics_api_active_websocket_count"].GetMetric()
	assert.Equal(t, 2, len(m), "should be 2 metric entries")
	// both m[0] and m[1] should have the value of 1
	assert.Equal(t, float64(2), m[0].GetGauge().GetValue())
	assert.Equal(t, float64(1), m[1].GetGauge().GetValue())

	// m[1] should have the subject of block
	labels = m[1].GetLabel()
	assert.Equal(t, "subject", labels[0].GetName())
	assert.Equal(t, "block", labels[0].GetValue())
}

func httpGet(t *testing.T, url string) ([]byte, int) {
	res, err := http.Get(url) //#nosec G107
	if err != nil {
		t.Fatal(err)
	}
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}
