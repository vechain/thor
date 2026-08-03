// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"os"
	"testing"

	"github.com/vechain/thor/v2/metrics"
)

func TestMain(m *testing.M) {
	// txpool metrics are lazy singletons. Initialize the Prometheus backend
	// before any test can resolve a metric against the default no-op backend.
	metrics.InitializePrometheusMetrics()
	os.Exit(m.Run())
}
