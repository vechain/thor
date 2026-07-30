// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"
)

func gatherByName(t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	out := make(map[string]*dto.MetricFamily)
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

func TestRegisterPullCollectorNoop(t *testing.T) {
	metrics = defaultNoopMetrics() // force the no-op service

	var calls atomic.Int64
	unregister := RegisterPullCollector("pulltest_noop", 0, func() []Sample {
		calls.Add(1)
		return []Sample{{Name: "value", Kind: KindGauge, Value: 1}}
	})
	require.NotNil(t, unregister)

	// Nothing should have been registered, and a scrape must not invoke the provider.
	_, present := gatherByName(t)["thor_metrics_pulltest_noop_value"]
	require.False(t, present)
	require.Equal(t, int64(0), calls.Load())

	require.NotPanics(t, unregister)
}

func TestPullCollectorTTLAndTypes(t *testing.T) {
	InitializePrometheusMetrics()

	var calls atomic.Int64
	unregister := RegisterPullCollector("pulltest_ttl", time.Hour, func() []Sample {
		n := calls.Add(1)
		return []Sample{
			{Name: "gauge_value", Kind: KindGauge, Value: float64(n)},
			{Name: "counter_value", Kind: KindCounter, Value: float64(10 * n)},
			{Name: "labelled", Kind: KindGauge, Labels: map[string]string{"k": "v"}, Value: float64(n)},
		}
	})
	defer unregister()

	// First scrape populates the cache.
	m := gatherByName(t)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, dto.MetricType_GAUGE, m["thor_metrics_pulltest_ttl_gauge_value"].GetType())
	require.Equal(t, dto.MetricType_COUNTER, m["thor_metrics_pulltest_ttl_counter_value"].GetType())
	require.Equal(t, float64(1), m["thor_metrics_pulltest_ttl_gauge_value"].Metric[0].GetGauge().GetValue())
	require.Equal(t, float64(10), m["thor_metrics_pulltest_ttl_counter_value"].Metric[0].GetCounter().GetValue())

	// Labels are preserved.
	labelled := m["thor_metrics_pulltest_ttl_labelled"].Metric[0]
	require.Equal(t, "k", labelled.Label[0].GetName())
	require.Equal(t, "v", labelled.Label[0].GetValue())

	// Second scrape within the TTL window must reuse the cache (provider not called again).
	m = gatherByName(t)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, float64(1), m["thor_metrics_pulltest_ttl_gauge_value"].Metric[0].GetGauge().GetValue())

	// After unregister the metric disappears.
	unregister()
	_, present := gatherByName(t)["thor_metrics_pulltest_ttl_gauge_value"]
	require.False(t, present)
}

func TestPullCollectorNoTTLRefreshesEveryScrape(t *testing.T) {
	InitializePrometheusMetrics()

	var calls atomic.Int64
	unregister := RegisterPullCollector("pulltest_nottl", 0, func() []Sample {
		n := calls.Add(1)
		return []Sample{{Name: "value", Kind: KindGauge, Value: float64(n)}}
	})
	defer unregister()

	// Registration warms up the provider once (call #1) to discover descriptors, so
	// with no TTL each subsequent scrape increments the value by one.
	m := gatherByName(t)
	require.Equal(t, float64(2), m["thor_metrics_pulltest_nottl_value"].Metric[0].GetGauge().GetValue())
	m = gatherByName(t)
	require.Equal(t, float64(3), m["thor_metrics_pulltest_nottl_value"].Metric[0].GetGauge().GetValue())
	require.Equal(t, int64(3), calls.Load())
}
