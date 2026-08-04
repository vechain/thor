// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package convert holds converters shared across api handler packages.
package convert

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/tx"
)

// MapSlice maps xs through fn, preserving nil.
func MapSlice[A, B any](xs []A, fn func(A) B) []B {
	if xs == nil {
		return nil
	}
	ys := make([]B, len(xs))
	for i, x := range xs {
		ys[i] = fn(x)
	}
	return ys
}

// ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) dto.Clause {
	return dto.Clause{
		To:    c.To(),
		Value: (*math.HexOrDecimal256)(c.Value()),
		Data:  hexutil.Encode(c.Data()),
	}
}
