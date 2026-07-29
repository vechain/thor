// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import "encoding/json"

// EthLogFilter mirrors the Ethereum eth_getLogs / eth_newFilter parameter object.
type EthLogFilter struct {
	FromBlock *string           `json:"fromBlock"`
	ToBlock   *string           `json:"toBlock"`
	Address   json.RawMessage   `json:"address"`   // string | []string | null
	Topics    []json.RawMessage `json:"topics"`    // each: null | string | []string
	BlockHash *string           `json:"blockHash"` // EIP-234: mutually exclusive with from/toBlock
}
