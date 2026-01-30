// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"github.com/vechain/thor/v2/metrics"
)

var (
	metricReceivedBlocksCount      = metrics.LazyLoadCounter("comm_sync_received_blocks_counter")
	metricFetchedBlockCount        = metrics.LazyLoadCounter("comm_sync_fetched_blocks_counter")
	metricReceivedTxsCount         = metrics.LazyLoadCounter("comm_sync_received_txs_counter")
	metricHandleRPCCounter         = metrics.LazyLoadCounterVec("comm_handle_rpc_counter", []string{"method", "error"})
	metricBlocksBroadcastedCounter = metrics.LazyLoadCounter("comm_blocks_broadcasted_counter")
)
