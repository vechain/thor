// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/vechain/thor/v2/metrics"
)

func TestCompactionSamples(t *testing.T) {
	stats := &leveldb.DBStats{
		LevelDurations:    []time.Duration{2 * time.Second, 4 * time.Second},
		LevelTablesCounts: []int{5, 9},
		LevelSizes:        []int64{100, 200},
		LevelRead:         []int64{1000, 2000},
		LevelWrite:        []int64{3000, 4000},
	}

	samples := compactionSamples(stats)
	// 2 levels * 5 types.
	require.Len(t, samples, 10)

	// Index by level+type for assertions.
	type key struct{ level, typ string }
	byKey := make(map[key]metrics.Sample)
	for _, s := range samples {
		byKey[key{s.Labels["level"], s.Labels["type"]}] = s
	}

	// Point-in-time values are gauges in the compaction_stats_gauge family.
	for _, typ := range []string{"tables", "size"} {
		s := byKey[key{"0", typ}]
		require.Equal(t, "compaction_stats_gauge", s.Name)
		require.Equal(t, metrics.KindGauge, s.Kind)
	}
	// Cumulative values are counters in the compaction_stats_total family.
	for _, typ := range []string{"time", "read", "write"} {
		s := byKey[key{"0", typ}]
		require.Equal(t, "compaction_stats_total", s.Name)
		require.Equal(t, metrics.KindCounter, s.Kind)
	}

	require.Equal(t, float64(5), byKey[key{"0", "tables"}].Value)
	require.Equal(t, float64(200), byKey[key{"1", "size"}].Value)
	require.Equal(t, float64(2), byKey[key{"0", "time"}].Value) // 2s -> 2
	require.Equal(t, float64(1000), byKey[key{"0", "read"}].Value)
	require.Equal(t, float64(4000), byKey[key{"1", "write"}].Value)
}
