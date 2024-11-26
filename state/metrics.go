package state

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricAccountCounter = metrics.LazyLoadCounterVec("account_write_count", []string{"type"})