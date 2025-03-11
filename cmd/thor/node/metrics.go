// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"math/big"
	"time"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/thor"
)

var (
	metricBlockProcessedCount    = metrics.LazyLoadCounterVec("block_processed_count", []string{"type", "success"})
	metricBlockProcessedTxs      = metrics.LazyLoadGaugeVec("block_processed_tx_gauge", []string{"type"})
	metricBlockProcessedGas      = metrics.LazyLoadGaugeVec("block_processed_gas_gauge", []string{"type"})
	metricBlockProcessedDuration = metrics.LazyLoadHistogram("block_processed_duration_ms", metrics.Bucket10s)
	metricChainForkCount         = metrics.LazyLoadCounter("chain_fork_count")
	metricBaseFeeGauge = metrics.LazyLoadGauge("base_fee_gauge")
)

func metricsRecordBaseFee(fc thor.ForkConfig, blk *block.Block) {
	// we only record base fee after GALACTICA
	if blk.Header().Number() < fc.GALACTICA {
		return
	}
	// if the node is not synced, we don't record the base fee
	if time.Unix(int64(blk.Header().Timestamp()), 0).Before(time.Now().Add(-time.Minute)) {
		return
	}

	baseFee := blk.Header().BaseFee()
	baseFeePercent := baseFee.Div(baseFee, big.NewInt(100))
	baseFeePercent = baseFeePercent.Div(baseFeePercent, big.NewInt(thor.InitialBaseFee))
	metricBaseFeeGauge().Set(baseFeePercent.Int64())
}
