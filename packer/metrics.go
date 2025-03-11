package packer

import "github.com/vechain/thor/v2/metrics"

var (
	metricTransactionTypeCounter = metrics.LazyLoadCounterVec("packer_transaction_type",  []string{"type"})
)
