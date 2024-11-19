package muxdb

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricCacheHitMissCounterVec = metrics.LazyLoadCounterVec("cache_hit_miss_count", []string{"type", "event"})