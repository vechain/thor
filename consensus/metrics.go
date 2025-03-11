package consensus

import "github.com/vechain/thor/v2/metrics"

var (
	metricTransactionTypeCounter = metrics.LazyLoadCounterVec("consensus_transaction_type",  []string{"type"})
	metricPriorityFeeBucket = metrics.LazyLoadHistogram("consensus_priority_fee_bucket", metrics.PriorityFeeBucket)
)
