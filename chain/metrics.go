package chain

import (
	"github.com/vechain/thor/v2/metrics"
)

var metricBlockWriteCounter = metrics.LazyLoadCounter("block_write_count")