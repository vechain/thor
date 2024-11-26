package state

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricAccountCounter = metrics.LazyLoadCounterVec("account_state_count", []string{"type", "target"})