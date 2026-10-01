// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package convert maps the node's internal objects to the api wire types and back.
package convert

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/tx"
)

// ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) api.Clause {
	return api.Clause{
		To:    c.To(),
		Value: (*math.HexOrDecimal256)(c.Value()),
		Data:  hexutil.Encode(c.Data()),
	}
}
