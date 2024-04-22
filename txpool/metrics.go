package txpool

import (
	"github.com/vechain/thor/v2/telemetry"
)

var metricTxPoolGauge = telemetry.LazyLoadGaugeVec("txpool_current_tx_count", []string{"source", "total"})
