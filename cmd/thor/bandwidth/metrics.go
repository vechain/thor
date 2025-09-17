package bandwidth

import "github.com/vechain/thor/v2/metrics"

var (
	metricGasPerSecond = metrics.LazyLoadGauge("bandwidth_gas_per_second")
	metricTimeElapsed  = metrics.LazyLoadGauge("bandwidth_time_elapsed_ms")
)
