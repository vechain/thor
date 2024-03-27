package txpool

import (
	"github.com/vechain/thor/v2/telemetry"
)

var metricTxPoolGauge = telemetry.LazyLoad(func() telemetry.GaugeVecMeter {
	return telemetry.GaugeVec("txpool_current_tx_count", []string{"source", "total"})
})
