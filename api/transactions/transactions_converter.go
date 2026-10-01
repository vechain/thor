// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/tx"
)

// ConvertTransaction convert a raw transaction into a json format transaction
func ConvertTransaction(trx *tx.Transaction, header *block.Header) *api.Transaction {
	// tx origin
	origin, _ := trx.Origin()
	delegator, _ := trx.Delegator()

	cls := make(api.Clauses, len(trx.Clauses()))
	for i, c := range trx.Clauses() {
		clause := api.ConvertClause(c)
		cls[i] = &clause
	}
	br := trx.BlockRef()
	t := &api.Transaction{
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
		t.GasPriceCoef = &coef
	default:
		t.MaxFeePerGas = (*math.HexOrDecimal256)(trx.MaxFeePerGas())
		t.MaxPriorityFeePerGas = (*math.HexOrDecimal256)(trx.MaxPriorityFeePerGas())
	}

	if header != nil {
		t.Meta = &api.TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		}
	}
	return t
}
