// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"fmt"
	"math/big"
	"time"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var (
	metricBlockProcessedCount    = metrics.LazyLoadCounterVec("block_processed_count", []string{"type", "success"})
	metricBlockProcessedTxs      = metrics.LazyLoadGaugeVec("block_processed_tx_gauge", []string{"type"})
	metricBlockProcessedGas      = metrics.LazyLoadGaugeVec("block_processed_gas_gauge", []string{"type"})
	metricBlockProcessedDuration = metrics.LazyLoadHistogram("block_processed_duration_ms", metrics.Bucket10s)
	metricChainForkCount         = metrics.LazyLoadCounter("chain_fork_count")
	metricTransactionTypeCounter = metrics.LazyLoadCounterVec("tx_type_counter", []string{"type"})
	metricPriorityFeeBucket      = metrics.LazyLoadHistogram("tx_priority_fee_bucket", []int64{0, 5, 10, 25, 100, 500, 1000, 10000, 100000, 1000000})
	metricBaseFeeGauge           = metrics.LazyLoadGauge("base_fee_gauge")
)

func metricsWriteTxData(fc thor.ForkConfig, blk *block.Block) {
	if blk.Header().Number() < fc.GALACTICA {
		return
	}
	if time.Unix(int64(blk.Header().Timestamp()), 0).Before(time.Now().Add(-time.Minute)) {
		return
	}

	// base fee
	baseFee := blk.Header().BaseFee()
	baseFeePercent := baseFee.Div(baseFee, big.NewInt(100))
	baseFeePercent = baseFeePercent.Div(baseFeePercent, big.NewInt(thor.InitialBaseFee))
	metricBaseFeeGauge().Set(baseFeePercent.Int64())

	// tx type & priority fee
	mapping := make(map[tx.Type]int64)
	for _, r := range blk.Transactions() {
		mapping[r.Type()]++
		if r.Type() == tx.TypeDynamicFee {
			metricPriorityFeeBucket().Observe(r.MaxPriorityFeePerGas().Int64())
		}
	}
	for t, c := range mapping {
		metricTransactionTypeCounter().AddWithLabel(c, map[string]string{"type": fmt.Sprintf("%d", t)})
	}
}
