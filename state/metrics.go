package state

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricAccountWriteCounter = metrics.LazyLoadCounterVec("account_write_count", []string{"type"})