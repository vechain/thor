// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/thor"
)

type TraceClauseOption struct {
	Name   string          `json:"name"`
	Target string          `json:"target"`
	Config json.RawMessage `json:"config"` // Config specific to given tracer.
}

type TraceCallOption struct {
	To         *thor.Address         `json:"to"`
	Value      *math.HexOrDecimal256 `json:"value"`
	Data       string                `json:"data"`
	Gas        uint64                `json:"gas"`
	GasPrice   *math.HexOrDecimal256 `json:"gasPrice"`
	ProvedWork *math.HexOrDecimal256 `json:"provedWork"`
	Caller     *thor.Address         `json:"caller"`
	GasPayer   *thor.Address         `json:"gasPayer"`
	Expiration uint32                `json:"expiration"`
	BlockRef   string                `json:"blockRef"`
	Name       string                `json:"name"`   // Tracer
	Config     json.RawMessage       `json:"config"` // Config specific to given tracer.
}

type StorageRangeOption struct {
	Address   thor.Address
	KeyStart  string
	MaxResult int
	Target    string
}

type StorageRangeResult struct {
	Storage StorageMap    `json:"storage"`
	NextKey *thor.Bytes32 `json:"nextKey"` // nil if Storage includes the last key in the trie.
}

type StorageMap map[string]StorageEntry

type StorageEntry struct {
	Key   *thor.Bytes32 `json:"key"`
	Value *thor.Bytes32 `json:"value"`
}
