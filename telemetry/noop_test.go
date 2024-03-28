package telemetry

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopTelemetry(t *testing.T) {
	server := httptest.NewServer(Handler())

	t.Cleanup(func() {
		server.Close()
	})

	// 2 ways of accessing it - useful to avoid lookups
	count1 := Counter("count1")
	Counter("count2")

	count1.Add(1)
	randCount2 := rand.Intn(100) + 1
	for i := 0; i < randCount2; i++ {
		Counter("count2").Add(1)
	}

	hist := Histogram("hist1", nil)
	histVect := HistogramVec("hist2", []string{"zeroOrOne"}, nil)
	for i := 0; i < rand.Intn(100)+1; i++ {
		hist.Observe(int64(i))
		histVect.ObserveWithLabels(int64(i), map[string]string{"thisIsNonsense": "butDoesntBreak"})
	}

	countVect := CounterVec("countVec1", []string{"zeroOrOne"})
	gaugeVec := GaugeVec("gaugeVec1", []string{"zeroOrOne"})
	for i := 0; i < rand.Intn(100)+1; i++ {
		countVect.AddWithLabel(int64(i), map[string]string{"thisIsNonsense": "butDoesntBreak"})
		gaugeVec.GaugeWithLabel(int64(i), map[string]string{"thisIsNonsense": "butDoesntBreak"})
	}

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Errorf("Failed to make GET request: %v", err)
	}

	defer resp.Body.Close()
	require.Equal(t, resp.StatusCode, 404)
}
