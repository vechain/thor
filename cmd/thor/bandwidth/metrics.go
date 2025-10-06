// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bandwidth

import "github.com/vechain/thor/v2/metrics"

var (
	metricGasPerSecond = metrics.LazyLoadGauge("bandwidth_gas_per_second")
	metricTimeElapsed  = metrics.LazyLoadGauge("bandwidth_time_elapsed_ms")
)
