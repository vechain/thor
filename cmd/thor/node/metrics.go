package node

import (
	"time"

	"github.com/vechain/thor/v2/telemetry"
)

var (
	metricBlockProposedDuration     = telemetry.HistogramVecWithHTTPBuckets("block_proposed_duration_ms", []string{"status"})
	metricBlockProposedCount        = telemetry.CounterVec("block_proposed_count", []string{"status"})
	metricBlockProposedTxs          = telemetry.CounterVec("block_proposed_tx_count", []string{"status"})
	metricBlockReceivedDuration     = telemetry.HistogramVecWithHTTPBuckets("block_received_duration_ms", []string{"status"})
	metricBlockReceivedCount        = telemetry.CounterVec("block_received_count", []string{"status"})
	metricBlockReceivedProcessedTxs = telemetry.CounterVec("block_received_processed_tx_count", []string{"status"})

	metricChainForkCount = telemetry.Counter("chain_fork_count")
	metricChainForkSize  = telemetry.Gauge("chain_fork_size")
)

func evalBlockReceivedMetrics(f func() error) error {
	startTime := time.Now()

	if err := f(); err != nil {
		status := map[string]string{
			"status": "failed",
		}
		metricBlockReceivedCount.AddWithLabel(1, status)
		metricBlockReceivedDuration.ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
		return err
	}

	status := map[string]string{
		"status": "proposed",
	}
	metricBlockReceivedCount.AddWithLabel(1, status)
	metricBlockReceivedDuration.ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
	return nil
}

// evalBlockProposeMetrics captures block proposing metrics
func evalBlockProposeMetrics(f func() error) error {
	startTime := time.Now()

	if err := f(); err != nil {
		status := map[string]string{
			"status": "failed",
		}
		metricBlockProposedCount.AddWithLabel(1, status)
		metricBlockProposedDuration.ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
		return err
	}

	status := map[string]string{
		"status": "proposed",
	}
	metricBlockProposedCount.AddWithLabel(1, status)
	metricBlockProposedDuration.ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
	return nil
}
