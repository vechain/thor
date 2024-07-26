// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopMetrics(t *testing.T) {
	server := httptest.NewServer(HTTPHandler())

	t.Cleanup(func() {
		server.Close()
	})

	// 2 ways of accessing it - useful to avoid lookups
	count1 := Counter("count1")
	Counter("count2")

	count1.Add(1)
	randCount2 := rand.Intn(100) + 1 // nolint:gosec
	for i := 0; i < randCount2; i++ {
		Counter("count2").Add(1)
	}

	hist := Histogram("hist1", nil)
	histVect := HistogramVec("hist2", []string{"zeroOrOne"}, nil)
	for i := 0; i < rand.Intn(100)+1; i++ { // nolint:gosec
		hist.Observe(int64(i))
		histVect.ObserveWithLabels(int64(i), map[string]string{"thisIsNonsense": "butDoesntBreak"})
	}

	countVect := CounterVec("countVec1", []string{"zeroOrOne"})
	gaugeVec := GaugeVec("gaugeVec1", []string{"zeroOrOne"})
	for i := 0; i < rand.Intn(100)+1; i++ { // nolint:gosec
		countVect.AddWithLabel(int64(i), map[string]string{"thisIsNonsense": "butDoesntBreak"})
		gaugeVec.AddWithLabel(int64(i), map[string]string{"thisIsNonsense": "butDoesntBreak"})
	}

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Errorf("Failed to make GET request: %v", err)
	}

	defer resp.Body.Close()
	require.Equal(t, resp.StatusCode, 404)
}
