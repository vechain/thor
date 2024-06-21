package subscriptions

import "github.com/vechain/thor/v2/metrics"

var (
	metricsActiveCount = metrics.LazyLoadGaugeVec("active_websocket_count", []string{"subject"})
)
