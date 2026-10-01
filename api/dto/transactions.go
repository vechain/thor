// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type RawTx struct {
	Raw string `json:"raw"`
}

func (rtx *RawTx) Decode() (*tx.Transaction, error) {
	data, err := hexutil.Decode(rtx.Raw)
	if err != nil {
		return nil, err
	}

	trx := new(tx.Transaction)
	if err := trx.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return trx, nil
}

type RawTransaction struct {
	RawTx
	Meta *TxMeta `json:"meta"`
}

type TxMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
}

type TransactionBase struct {
	ID                   thor.Bytes32          `json:"id"`
	Type                 uint8                 `json:"type"`
	ChainTag             byte                  `json:"chainTag"`
	BlockRef             string                `json:"blockRef"`
	Expiration           uint32                `json:"expiration"`
	Clauses              Clauses               `json:"clauses"`
	GasPriceCoef         *uint8                `json:"gasPriceCoef,omitempty"`
	Gas                  uint64                `json:"gas"`
	MaxFeePerGas         *math.HexOrDecimal256 `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *math.HexOrDecimal256 `json:"maxPriorityFeePerGas,omitempty"`
	Origin               thor.Address          `json:"origin"`
	Delegator            *thor.Address         `json:"delegator"`
	Nonce                math.HexOrDecimal64   `json:"nonce"`
	DependsOn            *thor.Bytes32         `json:"dependsOn"`
	Size                 uint32                `json:"size"`
}

type Transaction struct {
	TransactionBase
	Meta *TxMeta `json:"meta"`
}

type ReceiptMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
	TxID           thor.Bytes32 `json:"txID"`
	TxOrigin       thor.Address `json:"txOrigin"`
}

type Receipt struct {
	Type     uint8                 `json:"type,omitempty"`
	GasUsed  uint64                `json:"gasUsed"`
	GasPayer thor.Address          `json:"gasPayer"`
	Paid     *math.HexOrDecimal256 `json:"paid"`
	Reward   *math.HexOrDecimal256 `json:"reward"`
	Reverted bool                  `json:"reverted"`
	Meta     ReceiptMeta           `json:"meta"`
	Outputs  []*Output             `json:"outputs"`
}

// Output output of clause execution.
type Output struct {
	ContractAddress *thor.Address `json:"contractAddress"`
	Events          []*Event      `json:"events"`
	Transfers       []*Transfer   `json:"transfers"`
}

// SendTxResult is the response to the Send Tx method
type SendTxResult struct {
	ID *thor.Bytes32 `json:"id"`
}
