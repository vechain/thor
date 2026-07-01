// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// FeeHistoryParams holds the parameters for eth_feeHistory.
// RewardPercentiles is optional and defaults to nil when omitted or null.
type FeeHistoryParams struct {
	BlockCount        uint64
	NewestBlock       string
	RewardPercentiles []float64
}

func (p *FeeHistoryParams) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil || len(raws) < 2 {
		return fmt.Errorf("expected [blockCount, newestBlock, rewardPercentiles?]")
	}

	// blockCount may arrive as a hex string ("0xa") or a plain integer (10).
	var hexStr string
	if err := json.Unmarshal(raws[0], &hexStr); err == nil {
		n, err := hexutil.DecodeUint64(hexStr)
		if err != nil {
			return fmt.Errorf("invalid blockCount")
		}
		p.BlockCount = n
	} else if err := json.Unmarshal(raws[0], &p.BlockCount); err != nil {
		return fmt.Errorf("invalid blockCount")
	}

	if err := json.Unmarshal(raws[1], &p.NewestBlock); err != nil {
		return fmt.Errorf("invalid newestBlock")
	}

	if len(raws) >= 3 && string(raws[2]) != "null" {
		if err := json.Unmarshal(raws[2], &p.RewardPercentiles); err != nil {
			return fmt.Errorf("invalid rewardPercentiles")
		}
	}
	return nil
}

// FeeHistoryResult is the response type for eth_feeHistory.
// Reward is omitted when no reward percentiles were requested.
type FeeHistoryResult struct {
	OldestBlock   hexutil.Uint64   `json:"oldestBlock"`
	BaseFeePerGas []*hexutil.Big   `json:"baseFeePerGas"`
	GasUsedRatio  []float64        `json:"gasUsedRatio"`
	Reward        [][]*hexutil.Big `json:"reward,omitempty"`
}
