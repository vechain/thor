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

var (
	metricCacheHitMiss = metrics.LazyLoadGaugeVec("cache_hit_miss_count", []string{"type", "event"})
	metricCompaction   = metrics.LazyLoadGaugeVec("compaction_stats_gauge", []string{"level", "type"})
)

func registerCompactionMetrics(stats *leveldb.DBStats) {
	for i := range stats.LevelDurations {
		lvl := strconv.Itoa(i)
		metricCompaction().SetWithLabel(int64(stats.LevelTablesCounts[i]), map[string]string{"level": lvl, "type": "tables"})
		metricCompaction().SetWithLabel(stats.LevelSizes[i], map[string]string{"level": lvl, "type": "size"})
		metricCompaction().SetWithLabel(int64(stats.LevelDurations[i].Seconds()), map[string]string{"level": lvl, "type": "time"})
		metricCompaction().SetWithLabel(stats.LevelRead[i], map[string]string{"level": lvl, "type": "read"})
		metricCompaction().SetWithLabel(stats.LevelWrite[i], map[string]string{"level": lvl, "type": "write"})
	}
}
