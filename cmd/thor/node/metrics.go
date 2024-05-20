// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"github.com/vechain/thor/v2/metrics"
)

var (
	metricBlockProcessedCount    = metrics.LazyLoadCounterVec("block_processed_count", []string{"type", "success"})
	metricBlockProcessedTxs      = metrics.LazyLoadCounter("block_processed_tx_count")
	metricBlockProcessedGas      = metrics.LazyLoadCounter("block_processed_gas_count")
	metricBlockProcessedDuration = metrics.LazyLoadHistogram(
		"block_processed_duration_ms", metrics.Bucket10s,
	)
	metricChainForkCount = metrics.LazyLoadCounter("chain_fork_count")
	metricChainForkSize  = metrics.LazyLoadGauge("chain_fork_size")

	labelsProposed      = map[string]string{"type": "proposed", "success": "true"}
	labelsReceived      = map[string]string{"type": "received", "success": "true"}
	labelsProposeFailed = map[string]string{"type": "proposed", "success": "false"}
	labelsReceiveFailed = map[string]string{"type": "received", "success": "false"}
)
