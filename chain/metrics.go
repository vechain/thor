package chain

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricBlockRepositoryCounter = metrics.LazyLoadCounterVec("block_repository_count", []string{"type", "target"})

var metricTransactionRepositoryCounter = metrics.LazyLoadCounterVec("transaction_repository_count", []string{"type"})

var metricReceiptRepositoryCounter = metrics.LazyLoadCounterVec("receipt_repository_count", []string{"type"})