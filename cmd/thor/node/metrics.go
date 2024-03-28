package node

import (
	"time"

	"github.com/vechain/thor/v2/telemetry"
)

var (
	metricBlockProposedDuration = telemetry.LazyLoadHistogramVecWithHTTPBuckets(
		"block_proposed_duration_ms", []string{"status"},
	)
	metricBlockProposedCount = telemetry.LazyLoadCounterVec("block_proposed_count", []string{"status"})

	metricBlockProposedTxs      = telemetry.LazyLoadCounterVec("block_proposed_tx_count", []string{"status"})
	metricBlockReceivedDuration = telemetry.LazyLoadHistogramVecWithHTTPBuckets(
		"block_received_duration_ms", []string{"status"},
	)
	metricBlockReceivedCount        = telemetry.LazyLoadCounterVec("block_received_count", []string{"status"})
	metricBlockReceivedProcessedTxs = telemetry.LazyLoadCounterVec("block_received_processed_tx_count", []string{"status"})

	metricChainForkCount = telemetry.LazyLoadCounter("chain_fork_count")
	metricChainForkSize  = telemetry.LazyLoadGauge("chain_fork_size")
)

func evalBlockReceivedMetrics(f func() error) error {
	startTime := time.Now()

	if err := f(); err != nil {
		status := map[string]string{
			"status": "failed",
		}
		metricBlockReceivedCount().AddWithLabel(1, status)
		metricBlockReceivedDuration().ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
		return err
	}

	status := map[string]string{
		"status": "proposed",
	}
	metricBlockReceivedCount().AddWithLabel(1, status)
	metricBlockReceivedDuration().ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
	return nil
}

// evalBlockProposeMetrics captures block proposing metrics
func evalBlockProposeMetrics(f func() error) error {
	startTime := time.Now()

	if err := f(); err != nil {
		status := map[string]string{
			"status": "failed",
		}
		metricBlockProposedCount().AddWithLabel(1, status)
		metricBlockProposedDuration().ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
		return err
	}

	status := map[string]string{
		"status": "proposed",
	}
	metricBlockProposedCount().AddWithLabel(1, status)
	metricBlockProposedDuration().ObserveWithLabels(time.Since(startTime).Milliseconds(), status)
	return nil
}
