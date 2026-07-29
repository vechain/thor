// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// CallArgs mirrors the Ethereum eth_call / eth_estimateGas parameter object.
type CallArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Data                 hexutil.Bytes   `json:"data"`
}

// CallParams holds the arguments for eth_call and eth_estimateGas.
// Block is optional and defaults to "latest" when omitted or null; it accepts
// the same forms as Ethereum's BlockNumberOrHash.
type CallParams struct {
	Args  CallArgs
	Block BlockNumberOrHash
}

func (p *CallParams) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil || len(raws) < 1 {
		return fmt.Errorf("expected [callArgs, blockNrOrHash?]")
	}
	if err := json.Unmarshal(raws[0], &p.Args); err != nil {
		return fmt.Errorf("invalid call arguments: %w", err)
	}
	p.Block = LatestBlockNumberOrHash()
	if len(raws) >= 2 && string(raws[1]) != "null" {
		if err := json.Unmarshal(raws[1], &p.Block); err != nil {
			return fmt.Errorf("invalid block parameter: %w", err)
		}
	}
	return nil
}
