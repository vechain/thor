// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// #nosec G404
package metrics

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"
)

func TestPromMetrics(t *testing.T) {
	InitializePrometheusMetrics()

	// 2 ways of accessing it - useful to avoid lookups
	count1 := Counter("count1")
	Counter("count2")
	countVect := CounterVec("countVec1", []string{"zeroOrOne"})

	hist := Histogram("hist1", nil)
	HistogramVec("hist2", []string{"zeroOrOne"}, nil)

	gauge1 := Gauge("gauge1")
	gaugeVec := GaugeVec("gaugeVec1", []string{"zeroOrOne"})

	count1.Add(1)
	randCount2 := rand.N(100) + 1
	for range randCount2 {
		Counter("count2").Add(1)
	}

	histTotal := 0
	for i := range rand.N(100) + 2 {
		zeroOrOne := i % 2
		hist.Observe(int64(i))
		HistogramVec("hist2", []string{"zeroOrOne"}, nil).
			ObserveWithLabels(int64(i), map[string]string{"zeroOrOne": strconv.Itoa(zeroOrOne)})
		histTotal += i
	}

	totalCountVec := 0
	randCountVec := rand.N(100) + 2
	for i := range randCountVec {
		zeroOrOne := i % 2
		countVect.AddWithLabel(int64(i), map[string]string{"zeroOrOne": strconv.Itoa(zeroOrOne)})
		totalCountVec += i
	}

	totalGaugeVec := 0
	randGaugeVec := rand.N(100) + 2
	for i := range randGaugeVec {
		zeroOrOne := i % 2
		gaugeVec.AddWithLabel(int64(i), map[string]string{"zeroOrOne": strconv.Itoa(zeroOrOne)})
		gauge1.Add(int64(i))
		totalGaugeVec += i
	}

	// Gather the metrics
	gatherers := prometheus.Gatherers{prometheus.DefaultGatherer}
	metricFamilies, err := gatherers.Gather()
	require.NoError(t, err)

	metrics := make(map[string]*dto.MetricFamily)
	for _, mf := range metricFamilies {
		metrics[mf.GetName()] = mf
	}

	// Validate metrics
	require.Equal(t, float64(1), metrics["thor_metrics_count1"].Metric[0].GetCounter().GetValue())
	require.Equal(t, float64(randCount2), metrics["thor_metrics_count2"].Metric[0].GetCounter().GetValue())
	require.Equal(t, float64(histTotal), metrics["thor_metrics_hist1"].Metric[0].GetHistogram().GetSampleSum())

	sumHistVect := metrics["thor_metrics_hist2"].Metric[0].GetHistogram().GetSampleSum() +
		metrics["thor_metrics_hist2"].Metric[1].GetHistogram().GetSampleSum()
	require.Equal(t, float64(histTotal), sumHistVect)

	sumCountVec := metrics["thor_metrics_countVec1"].Metric[0].GetCounter().GetValue() +
		metrics["thor_metrics_countVec1"].Metric[1].GetCounter().GetValue()
	require.Equal(t, float64(totalCountVec), sumCountVec)

	require.Equal(t, float64(totalGaugeVec), metrics["thor_metrics_gauge1"].Metric[0].GetGauge().GetValue())
	sumGaugeVec := metrics["thor_metrics_gaugeVec1"].Metric[0].GetGauge().GetValue() +
		metrics["thor_metrics_gaugeVec1"].Metric[1].GetGauge().GetValue()
	require.Equal(t, float64(totalGaugeVec), sumGaugeVec)
}

func TestLazyLoading(t *testing.T) {
	metrics = defaultNoopMetrics() // make sure it starts in the default state of noopMeter

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

	lazyGauge := LazyLoadGauge("lazyGauge")
	lazyGaugeVec := LazyLoadGaugeVec("lazyGaugeVec", nil)
	lazyCounter := LazyLoadCounter("lazyCounter")
	lazyCounterVec := LazyLoadCounterVec("lazyCounterVec", nil)
	lazyHistogram := LazyLoadHistogram("lazyHistogram", nil)
	lazyHistogramVec := LazyLoadHistogramVec("lazyHistogramVec", nil, nil)

	// after initialization, newly created metrics become of the prometheus type
	InitializePrometheusMetrics()

	require.IsType(t, &promGaugeMeter{}, lazyGauge())
	require.IsType(t, &promGaugeVecMeter{}, lazyGaugeVec())
	require.IsType(t, &promCountMeter{}, lazyCounter())
	require.IsType(t, &promCountVecMeter{}, lazyCounterVec())
	require.IsType(t, &promHistogramMeter{}, lazyHistogram())
	require.IsType(t, &promHistogramVecMeter{}, lazyHistogramVec())
}
