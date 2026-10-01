// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/tx"
)

// ConvertTransactionBase converts a domain transaction into its wire base fields.
func ConvertTransactionBase(trx *tx.Transaction) dto.TransactionBase {
	origin, _ := trx.Origin()
	delegator, _ := trx.Delegator()

	clauses := trx.Clauses()
	cls := make(dto.Clauses, len(clauses))
	for i, c := range clauses {
		cls[i] = &dto.Clause{
			To:    c.To(),
			Value: (*math.HexOrDecimal256)(c.Value()),
			Data:  hexutil.Encode(c.Data()),
		}
	}

	br := trx.BlockRef()
	base := dto.TransactionBase{
		ChainTag:   trx.ChainTag(),
		Type:       trx.Type(),
		ID:         trx.ID(),
		Origin:     origin,
		BlockRef:   hexutil.Encode(br[:]),
		Expiration: trx.Expiration(),
		Nonce:      math.HexOrDecimal64(trx.Nonce()),
		Size:       uint32(trx.Size()),
		Gas:        trx.Gas(),
		DependsOn:  trx.DependsOn(),
		Clauses:    cls,
		Delegator:  delegator,
	}

	switch trx.Type() {
	case tx.TypeLegacy:
		coef := trx.GasPriceCoef()
		base.GasPriceCoef = &coef
	default:
		base.MaxFeePerGas = (*math.HexOrDecimal256)(trx.MaxFeePerGas())
		base.MaxPriorityFeePerGas = (*math.HexOrDecimal256)(trx.MaxPriorityFeePerGas())
	}

	return base
}
