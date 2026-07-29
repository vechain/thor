// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethconvert

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// LogCriteria is the parsed form of a log filter for fast per-event matching
// during incremental block scanning. Only TypeEthDynamicFee transaction events
// are matched. topics[i] holds all accepted values for position i; empty means
// wildcard. Adjacent positions are ANDed; alternatives within one position are ORed.
type LogCriteria struct {
	Addresses []thor.Address
	Topics    [5][]thor.Bytes32
}

func (c *LogCriteria) matchesEvent(e *tx.Event) bool {
	if len(c.Addresses) > 0 && !slices.Contains(c.Addresses, e.Address) {
		return false
	}
	for i, alts := range c.Topics {
		if len(alts) == 0 {
			continue // wildcard
		}
		if i >= len(e.Topics) {
			return false
		}
		if !slices.Contains(alts, e.Topics[i]) {
			return false
		}
	}
	return true
}

// ParseLogCriteria parses the address and topic fields from an EthLogFilter into a LogCriteria.
func ParseLogCriteria(f rpc.EthLogFilter) (LogCriteria, error) {
	var c LogCriteria

	if len(f.Address) > 0 && string(f.Address) != "null" {
		var single string
		var multi []string
		if err := json.Unmarshal(f.Address, &single); err == nil {
			addr, err := thor.ParseAddress(single)
			if err != nil {
				return c, fmt.Errorf("invalid address: %w", err)
			}
			c.Addresses = append(c.Addresses, addr)
		} else if err := json.Unmarshal(f.Address, &multi); err == nil {
			for _, s := range multi {
				addr, err := thor.ParseAddress(s)
				if err != nil {
					return c, fmt.Errorf("invalid address: %w", err)
				}
				c.Addresses = append(c.Addresses, addr)
			}
		}
	}

	topics := f.Topics
	if len(topics) > len(c.Topics) {
		topics = topics[:len(c.Topics)]
	}
	for i, raw := range topics {
		if raw == nil || string(raw) == "null" {
			continue
		}
		var single string
		var multi []string
		if err := json.Unmarshal(raw, &single); err == nil {
			h32, err := rpc.ParseBytes32Compact(single)
			if err != nil {
				return c, fmt.Errorf("invalid topic: %w", err)
			}
			c.Topics[i] = []thor.Bytes32{h32}
		} else if err := json.Unmarshal(raw, &multi); err == nil && len(multi) > 0 {
			alts := make([]thor.Bytes32, 0, len(multi))
			for _, s := range multi {
				h32, err := rpc.ParseBytes32Compact(s)
				if err != nil {
					return c, fmt.Errorf("invalid topic: %w", err)
				}
				alts = append(alts, h32)
			}
			c.Topics[i] = alts
		}
	}
	return c, nil
}

// CollectMatchingLogs scans ETH-typed transactions in a single block and returns rpc.EthLog
// entries matching the criteria. Projected transactionIndex and logIndex are relative to
// ETH-typed transactions only, consistent with eth_getTransactionByHash etc.
// Pass removed=true for blocks from a reorg (Obsolete=true) to set Removed on each log.
func CollectMatchingLogs(criteria *LogCriteria, txs tx.Transactions, receipts tx.Receipts, blockHash common.Hash, blockNum uint64, removed bool) []*rpc.EthLog {
	var logs []*rpc.EthLog
	var projEthIdx uint64
	var projLogIdx uint64

	for i, t := range txs {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		receipt := receipts[i]
		if len(receipt.Outputs) > 0 {
			for j, event := range receipt.Outputs[0].Events {
				if criteria.matchesEvent(event) {
					topics := make([]common.Hash, len(event.Topics))
					for k, tp := range event.Topics {
						topics[k] = common.Hash(tp)
					}
					logs = append(logs, &rpc.EthLog{
						Address:     common.Address(event.Address),
						Topics:      topics,
						Data:        event.Data,
						BlockNumber: hexutil.Uint64(blockNum),
						TxHash:      common.Hash(t.ID()),
						TxIndex:     hexutil.Uint64(projEthIdx),
						BlockHash:   blockHash,
						LogIndex:    hexutil.Uint64(projLogIdx + uint64(j)),
						Removed:     removed,
					})
				}
			}
			projLogIdx += uint64(len(receipt.Outputs[0].Events))
		}
		projEthIdx++
	}
	return logs
}
