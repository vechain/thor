// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/api/types"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type Transaction struct {
	ID                   thor.Bytes32          `json:"id"`
	Type                 uint8                 `json:"type"`
	ChainTag             byte                  `json:"chainTag"`
	BlockRef             string                `json:"blockRef"`
	Expiration           uint32                `json:"expiration"`
	Clauses              types.Clauses         `json:"clauses"`
	GasPriceCoef         *uint8                `json:"gasPriceCoef,omitempty"`
	Gas                  uint64                `json:"gas"`
	MaxFeePerGas         *math.HexOrDecimal256 `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *math.HexOrDecimal256 `json:"maxPriorityFeePerGas,omitempty"`
	Origin               thor.Address          `json:"origin"`
	Delegator            *thor.Address         `json:"delegator"`
	Nonce                math.HexOrDecimal64   `json:"nonce"`
	DependsOn            *thor.Bytes32         `json:"dependsOn"`
	Size                 uint32                `json:"size"`
	Meta                 *types.TxMeta         `json:"meta"`
}

// ConvertTransaction convert a raw transaction into a json format transaction
func ConvertTransaction(trx *tx.Transaction, header *block.Header) *Transaction {
	//tx origin
	origin, _ := trx.Origin()
	delegator, _ := trx.Delegator()

	cls := make(types.Clauses, len(trx.Clauses()))
	for i, c := range trx.Clauses() {
		clause := types.ConvertClause(c)
		cls[i] = &clause
	}
	br := trx.BlockRef()
	t := &Transaction{
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
		t.Meta = &types.TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		}
	}
	return t
}
