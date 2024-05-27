// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func TestMetricsMiddleware(t *testing.T) {
	metrics.InitializePrometheusMetrics()

	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	router := mux.NewRouter()
	acc := accounts.New(repo, stater, math.MaxUint64, thor.NoFork, solo.NewBFTEngine(repo))
	acc.Mount(router, "/accounts")
	router.PathPrefix("/metrics").Handler(metrics.HTTPHandler())
	router.Use(metricsMiddleware)
	ts := httptest.NewServer(router)

	httpGet(t, ts.URL+"/accounts/0x")
	httpGet(t, ts.URL+"/accounts/"+thor.Address{}.String())

	body, _ := httpGet(t, ts.URL+"/metrics")
	parser := expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	assert.Nil(t, err)

	m := metrics["thor_metrics_api_request_count"].GetMetric()
	assert.Equal(t, 2, len(m), "should be 2 metric entries")
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
}

func httpGet(t *testing.T, url string) ([]byte, int) {
	res, err := http.Get(url) // nolint:gosec
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
