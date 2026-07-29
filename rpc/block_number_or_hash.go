// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// BlockNumberOrHash mirrors go-ethereum's rpc.BlockNumberOrHash. It accepts either:
//   - a string: a tag ("latest", "earliest", "pending", "safe", "finalized"),
//     a hex block number ("0x1"), or a 32-byte block hash ("0x" + 64 hex chars)
//   - an object: {"blockNumber": "...", "blockHash": "...", "requireCanonical": bool}
//
// At most one of BlockNumber and BlockHash is non-nil; when both are nil the
// caller should treat the value as the default "latest".
type BlockNumberOrHash struct {
	BlockNumber      *string
	BlockHash        *common.Hash
	RequireCanonical bool
}

type bnhObject struct {
	BlockNumber      *string      `json:"blockNumber,omitempty"`
	BlockHash        *common.Hash `json:"blockHash,omitempty"`
	RequireCanonical bool         `json:"requireCanonical,omitempty"`
}

func (b *BlockNumberOrHash) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var e bnhObject
		if err := json.Unmarshal(data, &e); err != nil {
			return err
		}
		if e.BlockNumber != nil && e.BlockHash != nil {
			return errors.New("cannot specify both BlockHash and BlockNumber, choose one or the other")
		}
		b.BlockNumber = e.BlockNumber
		b.BlockHash = e.BlockHash
		b.RequireCanonical = e.RequireCanonical
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if len(s) == 66 && strings.HasPrefix(s, "0x") {
		var h common.Hash
		if err := h.UnmarshalText([]byte(s)); err != nil {
			return err
		}
		b.BlockHash = &h
		return nil
	}
	b.BlockNumber = &s
	return nil
}

// LatestBlockNumberOrHash returns a BlockNumberOrHash that resolves to the latest block.
// It is used as the default value when callers omit the block parameter.
func LatestBlockNumberOrHash() BlockNumberOrHash {
	latest := "latest"
	return BlockNumberOrHash{BlockNumber: &latest}
}
