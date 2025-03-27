// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/thor"
)

type FeesHistory struct {
	OldestBlock   thor.Bytes32   `json:"oldestBlock"`
	BaseFeePerGas []*hexutil.Big `json:"baseFeePerGas"`
	GasUsedRatio  []float64      `json:"gasUsedRatio"`
}

type FeesPriority struct {
	MaxPriorityFeePerGas *hexutil.Big `json:"maxPriorityFeePerGas"`
}
