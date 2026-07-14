// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"database/sql"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"

	"github.com/vechain/thor/v2/metrics"
)

func TestReadDBStatsSamples(t *testing.T) {
	samples := readDBStatsSamples(sql.DBStats{
		InUse:        3,
		WaitCount:    7,
		WaitDuration: 250 * time.Millisecond,
	})
	require.Len(t, samples, 3)

	byName := make(map[string]metrics.Sample)
	for _, s := range samples {
		byName[s.Name] = s
	}

	require.Equal(t, metrics.KindGauge, byName["readdb_in_use_connections"].Kind)
	require.Equal(t, float64(3), byName["readdb_in_use_connections"].Value)

	require.Equal(t, metrics.KindCounter, byName["readdb_wait_count"].Kind)
	require.Equal(t, float64(7), byName["readdb_wait_count"].Value)

	require.Equal(t, metrics.KindCounter, byName["readdb_wait_duration_ms"].Kind)
	require.Equal(t, float64(250), byName["readdb_wait_duration_ms"].Value)
}

func TestEnableMetricsExportsReadDBPoolStats(t *testing.T) {
	metrics.InitializePrometheusMetrics()

	db, err := NewMem()
	require.NoError(t, err)
	db.EnableMetrics()

	gather := func() map[string]*dto.MetricFamily {
		mfs, err := prometheus.DefaultGatherer.Gather()
		require.NoError(t, err)
		out := make(map[string]*dto.MetricFamily)
		for _, mf := range mfs {
			out[mf.GetName()] = mf
		}
		return out
	}

	m := gather()
	require.Equal(t, dto.MetricType_GAUGE, m["thor_metrics_logdb_readdb_in_use_connections"].GetType())
	require.Equal(t, dto.MetricType_COUNTER, m["thor_metrics_logdb_readdb_wait_count"].GetType())
	require.Equal(t, dto.MetricType_COUNTER, m["thor_metrics_logdb_readdb_wait_duration_ms"].GetType())

	// Closing the DB must release the collector.
	require.NoError(t, db.Close())
	_, present := gather()["thor_metrics_logdb_readdb_in_use_connections"]
	require.False(t, present)
}
