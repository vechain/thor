// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"time"

	"github.com/vechain/thor/v2/telemetry"
)

var (
	metricBlockProposedCount    = telemetry.LazyLoadCounterVec("block_proposed_count", []string{"status"})
	metricBlockProposedTxs      = telemetry.LazyLoadCounterVec("block_proposed_tx_count", []string{"status"})
	metricBlockProposedDuration = telemetry.LazyLoadHistogramVec(
		"block_proposed_duration_ms", []string{"status"}, telemetry.Bucket10s,
	)

	metricBlockReceivedCount        = telemetry.LazyLoadCounterVec("block_received_count", []string{"status"})
	metricBlockReceivedProcessedTxs = telemetry.LazyLoadCounterVec("block_received_processed_tx_count", []string{"status"})
	metricBlockReceivedDuration     = telemetry.LazyLoadHistogramVec(
		"block_received_duration_ms", []string{"status"}, telemetry.Bucket10s,
	)

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
		"status": "received",
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
