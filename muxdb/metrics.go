// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"strconv"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/vechain/thor/v2/metrics"
)

var metricCacheHitMiss = metrics.LazyLoadGaugeVec("cache_hit_miss_count", []string{"type", "event"})

// compactionSamples converts LevelDB per-level compaction stats into samples:
// tables/size are gauges, cumulative time/read/write are counters.
func compactionSamples(stats *leveldb.DBStats) []metrics.Sample {
	samples := make([]metrics.Sample, 0, len(stats.LevelDurations)*5)
	for i := range stats.LevelDurations {
		lvl := strconv.Itoa(i)
		gauge := func(typ string, value int64) {
			samples = append(samples, metrics.Sample{
				Name:   "compaction_stats_gauge",
				Help:   "LevelDB per-level compaction gauges (current tables count and size).",
				Kind:   metrics.KindGauge,
				Labels: map[string]string{"level": lvl, "type": typ},
				Value:  float64(value),
			})
		}
		counter := func(typ string, value int64) {
			samples = append(samples, metrics.Sample{
				Name:   "compaction_stats_total",
				Help:   "LevelDB per-level cumulative compaction counters (time seconds, read/write bytes).",
				Kind:   metrics.KindCounter,
				Labels: map[string]string{"level": lvl, "type": typ},
				Value:  float64(value),
			})
		}
		gauge("tables", int64(stats.LevelTablesCounts[i]))
		gauge("size", stats.LevelSizes[i])
		counter("time", int64(stats.LevelDurations[i].Seconds()))
		counter("read", stats.LevelRead[i])
		counter("write", stats.LevelWrite[i])
	}
	return samples
}
