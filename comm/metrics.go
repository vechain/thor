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
