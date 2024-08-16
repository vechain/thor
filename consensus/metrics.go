// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import "github.com/vechain/thor/v2/metrics"

var (
	metricCoefBucket = metrics.LazyLoadHistogram("consensus_coef_histogram", metrics.BucketCoefficients)
)
