// Copyright (c) 2025 The VeChainThor developers
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Common types used across multiple API modules

// Event represents a blockchain event
type Event struct {
	Address thor.Address   `json:"address"`
	Topics  []thor.Bytes32 `json:"topics"`
	Data    string         `json:"data"`
}

// Transfer represents a token transfer
type Transfer struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
}

// Clause represents a transaction clause
type Clause struct {
	To    *thor.Address         `json:"to"`
	Value *math.HexOrDecimal256 `json:"value"`
	Data  string                `json:"data"`
}

// Clauses array of clauses
type Clauses []*Clause

// LogMeta represents metadata for logs
type LogMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
	TxID           thor.Bytes32 `json:"txID"`
	TxOrigin       thor.Address `json:"txOrigin"`
	ClauseIndex    uint32       `json:"clauseIndex"`
	TxIndex        *uint32      `json:"txIndex,omitempty"`
	LogIndex       *uint32      `json:"logIndex,omitempty"`
}

// ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) Clause {
	return Clause{
		To:    c.To(),
		Value: (*math.HexOrDecimal256)(c.Value()),
		Data:  hexutil.Encode(c.Data()),
	}
}

func (c *Clause) String() string {
	return fmt.Sprintf(`Clause(
		To    %v
		Value %v
		Data  %v
		)`, c.To,
		c.Value,
		c.Data)
}
