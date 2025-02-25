// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type Transaction struct {
	ID                   thor.Bytes32          `json:"id"`
	TxType               math.HexOrDecimal64   `json:"txType"`
	ChainTag             byte                  `json:"chainTag"`
	BlockRef             string                `json:"blockRef"`
	Expiration           uint32                `json:"expiration"`
	Clauses              Clauses               `json:"clauses"`
	GasPriceCoef         uint8                 `json:"gasPriceCoef"`
	Gas                  uint64                `json:"gas"`
	MaxFeePerGas         *math.HexOrDecimal256 `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *math.HexOrDecimal256 `json:"maxPriorityFeePerGas,omitempty"`
	Origin               thor.Address          `json:"origin"`
	Delegator            *thor.Address         `json:"delegator"`
	Nonce                math.HexOrDecimal64   `json:"nonce"`
	DependsOn            *thor.Bytes32         `json:"dependsOn"`
	Size                 uint32                `json:"size"`
	Meta                 *TxMeta               `json:"meta"`
}

// convertTransaction convert a raw transaction into a json format transaction
func convertTransaction(trx *tx.Transaction, header *block.Header) *Transaction {
	//tx origin
	origin, _ := trx.Origin()
	delegator, _ := trx.Delegator()

	cls := make(Clauses, len(trx.Clauses()))
	for i, c := range trx.Clauses() {
		cls[i] = convertClause(c)
	}
	br := trx.BlockRef()
	t := &Transaction{
		ChainTag:   trx.ChainTag(),
		TxType:     math.HexOrDecimal64(trx.Type()),
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
		t.GasPriceCoef = trx.GasPriceCoef()
	default:
		t.MaxFeePerGas = (*math.HexOrDecimal256)(trx.MaxFeePerGas())
		t.MaxPriorityFeePerGas = (*math.HexOrDecimal256)(trx.MaxPriorityFeePerGas())
	}

	if header != nil {
		t.Meta = &TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		}
	}
	return t
}
