package consensus

import "github.com/vechain/thor/v2/metrics"

var (
	metricCoefBucket = metrics.LazyLoadHistogram("consensus_coef_histogram", metrics.BucketCoefficients)
)
