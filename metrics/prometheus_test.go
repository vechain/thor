// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestPromMetrics(t *testing.T) {
	noopGauge := Gauge("noopGauge")
	lazyLoadGauge := LazyLoadGauge("lazyGauge")
	InitializePrometheusMetrics()
	server := httptest.NewServer(HTTPHandler())

	t.Cleanup(func() {
		server.Close()
	})

	if _, ok := noopGauge.(*noopMeters); !ok {
		t.Error("noopGauge is not noopmetrics")
	}

	if _, ok := lazyLoadGauge().(*promGaugeMeter); !ok {
		t.Error("noopGauge is not promGaugeMeter")
	}

	// 2 ways of accessing it - useful to avoid lookups
	count1 := Counter("count1")
	Counter("count2")
	countVect := CounterVec("countVec1", []string{"zeroOrOne"})

	hist := Histogram("hist1", nil)
	HistogramVec("hist2", []string{"zeroOrOne"}, nil)

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
		HistogramVec("hist2", []string{"zeroOrOne"}, nil).
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
		gauge1.Add(int64(i))
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

	require.Equal(t, metrics["thor_metrics_count1"].GetMetric()[0].GetCounter().GetValue(), float64(1))
	require.Equal(t, metrics["thor_metrics_count2"].GetMetric()[0].GetCounter().GetValue(), float64(randCount2))
	require.Equal(t, metrics["thor_metrics_hist1"].GetMetric()[0].GetHistogram().GetSampleSum(), float64(histTotal))

	sumHistVect := metrics["thor_metrics_hist2"].GetMetric()[0].GetHistogram().GetSampleSum() +
		metrics["thor_metrics_hist2"].GetMetric()[1].GetHistogram().GetSampleSum()
	require.Equal(t, sumHistVect, float64(histTotal))

	sumCountVec := metrics["thor_metrics_countVec1"].GetMetric()[0].GetCounter().GetValue() +
		metrics["thor_metrics_countVec1"].GetMetric()[1].GetCounter().GetValue()
	require.Equal(t, sumCountVec, float64(totalCountVec))

	require.Equal(t, metrics["thor_metrics_gauge1"].GetMetric()[0].GetGauge().GetValue(), float64(totalGaugeVec))
	sumGaugeVec := metrics["thor_metrics_gaugeVec1"].GetMetric()[0].GetGauge().GetValue() +
		metrics["thor_metrics_gaugeVec1"].GetMetric()[1].GetGauge().GetValue()
	require.Equal(t, sumGaugeVec, float64(totalGaugeVec))
}

func TestLazyLoading(t *testing.T) {
	metrics = defaultNoopMetrics() // make sure it starts in the default state

	for _, a := range []any{
		Gauge("noopGauge"),
		GaugeVec("noopGauge", nil),
		Counter("noopCounter"),
		CounterVec("noopCounter", nil),
		Histogram("noopHist", nil),
		HistogramVec("noopHist", nil, nil),
	} {
		require.IsType(t, &noopMeters{}, a)
	}

	// after initialization, newly created metrics become of the prometheus type
	InitializePrometheusMetrics()

	require.IsType(t, &promGaugeMeter{}, LazyLoadGauge("lazyGauge")())
	require.IsType(t, &promGaugeVecMeter{}, LazyLoadGaugeVec("lazyGaugeVec", nil)())
	require.IsType(t, &promCountMeter{}, LazyLoadCounter("lazyCounter")())
	require.IsType(t, &promCountVecMeter{}, LazyLoadCounterVec("lazyCounterVec", nil)())
	require.IsType(t, &promHistogramMeter{}, LazyLoadHistogram("lazyHistogram", nil)())
	require.IsType(t, &promHistogramVecMeter{}, LazyLoadHistogramVec("lazyHistogramVec", nil, nil)())
}
