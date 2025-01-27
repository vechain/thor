// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"github.com/vechain/thor/v2/metrics"
)

var (
	metricBlockProcessedCount    = metrics.LazyLoadCounterVec("block_processed_count", []string{"type", "success"})
	metricBlockProcessedTxs      = metrics.LazyLoadGaugeVec("block_processed_tx_gauge", []string{"type"})
	metricBlockProcessedGas      = metrics.LazyLoadGaugeVec("block_processed_gas_gauge", []string{"type"})
	metricBlockProcessedDuration = metrics.LazyLoadHistogram("block_processed_duration_ms", metrics.Bucket10s)
	metricChainForkCount         = metrics.LazyLoadCounter("chain_fork_count")
)
