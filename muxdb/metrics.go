// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricCacheHitMiss = metrics.LazyLoadGaugeVec("cache_hit_miss_count", []string{"type", "event"})
