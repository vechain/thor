package telemetry

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestOtelPromTelemetry(t *testing.T) {
	InitializeOtelTelemetry()
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

	hist := Histogram("hist1")
	histTotal := 0
	for i := 0; i < rand.Intn(100)+1; i++ {
		hist.Observe(int64(i))
		histTotal += i
	}

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Errorf("Failed to make GET request: %v", err)
	}

	defer resp.Body.Close()

	parser := expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(resp.Body)
	require.NoError(t, err)

	require.Equal(t, metrics["count1_total"].GetMetric()[0].GetCounter().GetValue(), float64(1))
	require.Equal(t, metrics["count2_total"].GetMetric()[0].GetCounter().GetValue(), float64(randCount2))
	require.Equal(t, metrics["hist1"].GetMetric()[0].GetHistogram().GetSampleSum(), float64(histTotal))
}
