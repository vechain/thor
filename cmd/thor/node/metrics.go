// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"time"

	"github.com/vechain/thor/v2/thor"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/tx"
)

var (
	metricBlockProcessedCount    = metrics.LazyLoadCounterVec("block_processed_count", []string{"type", "success"})
	metricBlockProcessedTxs      = metrics.LazyLoadGaugeVec("block_processed_tx_gauge", []string{"type"})
	metricBlockProcessedGas      = metrics.LazyLoadGaugeVec("block_processed_gas_gauge", []string{"type"})
	metricBlockProcessedClauses  = metrics.LazyLoadGaugeVec("block_processed_clauses_gauge", []string{"type"})
	metricBlockGasUsage          = metrics.LazyLoadGauge("block_gas_usage_gauge")
	metricBlockSize              = metrics.LazyLoadGauge("block_size_gauge")
	metricsBlockGapTimeGauge     = metrics.LazyLoadGauge("block_gap_time_gauge")
	metricBlockProcessedDuration = metrics.LazyLoadHistogram("block_processed_duration_ms", metrics.Bucket10s)
	metricChainForkCount         = metrics.LazyLoadCounter("chain_fork_count")
	metricChainForkSize          = metrics.LazyLoadGauge("chain_fork_gauge")
	metricChainForkBlocks        = metrics.LazyLoadCounterVec("chain_fork_blocks_count", []string{"number"})
	metricMissedSlotCount        = metrics.LazyLoadCounter("block_missed_slot_count")
)

func recordBlockMetrics(newBlock *block.Block, oldBest *chain.BlockSummary, receipts tx.Receipts, realElapsed mclock.AbsTime, proposed bool) {
	labels := make(map[string]string)
	if proposed {
		labels["type"] = "proposed"
	} else {
		labels["type"] = "received"
	}

	metricBlockProcessedTxs().SetWithLabel(int64(len(receipts)), labels)
	metricBlockProcessedGas().SetWithLabel(int64(newBlock.Header().GasUsed()), labels)
	metricBlockProcessedDuration().Observe(time.Duration(realElapsed).Milliseconds())
	// skip block timings if we're processing a fork
	if oldBest.Header.ID() == newBlock.Header().ParentID() {
		blockGap := newBlock.Header().Timestamp() - oldBest.Header.Timestamp()
		metricsBlockGapTimeGauge().Set(int64(blockGap))
		if blockGap > thor.BlockInterval+5 {
			metricMissedSlotCount().Add(1)
		}
	}
	metricBlockGasUsage().Set(int64(newBlock.Header().GasUsed() * 100 / newBlock.Header().GasLimit()))
	metricBlockSize().Set(int64(newBlock.Size()))
	processedClauses := 0
	for _, receipt := range receipts {
		processedClauses += len(receipt.Outputs)
	}
	metricBlockProcessedClauses().SetWithLabel(int64(processedClauses), labels)
}
