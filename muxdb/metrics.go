package muxdb

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricCacheHitMissGauge = metrics.LazyLoadGaugeVec("cache_hit_miss_count", []string{"type"})