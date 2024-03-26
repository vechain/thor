package telemetry

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestOtelPromTelemetry(t *testing.T) {
	InitializePrometheusTelemetry()
	server := httptest.NewServer(Handler())

	t.Cleanup(func() {
		server.Close()
	})

	// 2 ways of accessing it - useful to avoid lookups
	count1 := Counter("count1")
	Counter("count2")
	countVect := CounterVec("countVec1", []string{"zeroOrOne"})

	hist := HistogramWithHTTPBuckets("hist1")
	HistogramVec("hist2", []string{"zeroOrOne"})

	gauge1 := Gauge("gauge1")
	gaugeVec := GaugeVec("gaugeVec1", []string{"zeroOrOne"})

	count1.Add(1)
	randCount2 := rand.Intn(100) + 1
	for i := 0; i < randCount2; i++ {
		Counter("count2").Add(1)
	}

	histTotal := 0
	for i := 0; i < rand.Intn(100)+1; i++ {
		zeroOrOne := i % 2
		hist.Observe(int64(i))
		HistogramVec("hist2", []string{"zeroOrOne"}).
			ObserveWithLabels(int64(i), map[string]string{"zeroOrOne": strconv.Itoa(zeroOrOne)})
		histTotal += i
	}

	totalCountVec := 0
	randCountVec := rand.Intn(100) + 1
	for i := 0; i < randCountVec; i++ {
		zeroOrOne := i % 2
		countVect.AddWithLabel(int64(i), map[string]string{"zeroOrOne": strconv.Itoa(zeroOrOne)})
		totalCountVec += i
	}

	totalGaugeVec := 0
	randGaugeVec := rand.Intn(100) + 1
	for i := 0; i < randGaugeVec; i++ {
		zeroOrOne := i % 2
		gaugeVec.GaugeWithLabel(int64(i), map[string]string{"zeroOrOne": strconv.Itoa(zeroOrOne)})
		gauge1.Gauge(int64(i))
		totalGaugeVec += i
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

	require.Equal(t, metrics["node_telemetry_count1"].GetMetric()[0].GetCounter().GetValue(), float64(1))
	require.Equal(t, metrics["node_telemetry_count2"].GetMetric()[0].GetCounter().GetValue(), float64(randCount2))
	require.Equal(t, metrics["node_telemetry_hist1"].GetMetric()[0].GetHistogram().GetSampleSum(), float64(histTotal))

	sumHistVect := metrics["node_telemetry_hist2"].GetMetric()[0].GetHistogram().GetSampleSum() +
		metrics["node_telemetry_hist2"].GetMetric()[1].GetHistogram().GetSampleSum()
	require.Equal(t, sumHistVect, float64(histTotal))

	sumCountVec := metrics["node_telemetry_countVec1"].GetMetric()[0].GetCounter().GetValue() +
		metrics["node_telemetry_countVec1"].GetMetric()[1].GetCounter().GetValue()
	require.Equal(t, sumCountVec, float64(totalCountVec))

	require.Equal(t, metrics["node_telemetry_gauge1"].GetMetric()[0].GetGauge().GetValue(), float64(totalGaugeVec))
	sumGaugeVec := metrics["node_telemetry_gaugeVec1"].GetMetric()[0].GetGauge().GetValue() +
		metrics["node_telemetry_gaugeVec1"].GetMetric()[1].GetGauge().GetValue()
	require.Equal(t, sumGaugeVec, float64(totalGaugeVec))
}
