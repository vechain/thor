package chain

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricBlockWriteCounter = metrics.LazyLoadCounter("block_write_count")
var metricBlockReadCounter = metrics.LazyLoadCounterVec("block_read_count", []string{"type"})

var metricTransactionWriteCounter = metrics.LazyLoadCounter("transaction_write_count")
var metricTransactionReadCounter = metrics.LazyLoadCounter("transaction_read_count")

var metricReceiptWriteCounter = metrics.LazyLoadCounter("receipt_write_count")
var metricReceiptReadCounter = metrics.LazyLoadCounter("receipt_read_count")