// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import "github.com/vechain/thor/v2/metrics"

var (
	metricCacheHitMiss = metrics.LazyLoadGaugeVec("repo_cache_hit_miss_count", []string{"type", "event"})
	metricBlockRepositoryCounter = metrics.LazyLoadCounterVec("block_repository_count", []string{"type", "target"})
	metricTransactionRepositoryCounter = metrics.LazyLoadCounterVec("transaction_repository_count", []string{"type", "target"})
	metricReceiptRepositoryCounter = metrics.LazyLoadCounterVec("receipt_repository_count", []string{"type", "target"})
)
