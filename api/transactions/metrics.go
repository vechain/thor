package transactions

import "github.com/vechain/thor/v2/metrics"

var (
	metricTransactionType = metrics.LazyLoadCounterVec("api_transaction_type_counter",  []string{"type"})
)
