// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/thor"
)

type Account struct {
	Balance *math.HexOrDecimal256 `json:"balance"`
	Energy  *math.HexOrDecimal256 `json:"energy"`
	HasCode bool                  `json:"hasCode"`
}

// CallData represents contract-call body
type CallData struct {
	Value    *math.HexOrDecimal256 `json:"value"`
	Data     string                `json:"data"`
	Gas      uint64                `json:"gas"`
	GasPrice *math.HexOrDecimal256 `json:"gasPrice"`
	Caller   *thor.Address         `json:"caller"`
}

type GetCodeResult struct {
	Code string `json:"code"`
}

type GetStorageResult struct {
	Value string `json:"value"`
}

type GetRawStorageResult struct {
	Value string `json:"value"`
}

type CallResult struct {
	Data      string      `json:"data"`
	Events    []*Event    `json:"events"`
	Transfers []*Transfer `json:"transfers"`
	GasUsed   uint64      `json:"gasUsed"`
	Reverted  bool        `json:"reverted"`
	VMError   string      `json:"vmError"`
}

// BatchCallData executes a batch of codes
type BatchCallData struct {
	Clauses    Clauses               `json:"clauses"`
	Gas        uint64                `json:"gas"`
	GasPrice   *math.HexOrDecimal256 `json:"gasPrice"`
	ProvedWork *math.HexOrDecimal256 `json:"provedWork"`
	Caller     *thor.Address         `json:"caller"`
	GasPayer   *thor.Address         `json:"gasPayer"`
	Expiration uint32                `json:"expiration"`
	BlockRef   string                `json:"blockRef"`
}

type BatchCallResults []*CallResult
