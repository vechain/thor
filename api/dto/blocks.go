// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/thor"
)

type BlockSummary struct {
	Number        uint32                `json:"number"`
	ID            thor.Bytes32          `json:"id"`
	Size          uint32                `json:"size"`
	ParentID      thor.Bytes32          `json:"parentID"`
	Timestamp     uint64                `json:"timestamp"`
	GasLimit      uint64                `json:"gasLimit"`
	Beneficiary   thor.Address          `json:"beneficiary"`
	GasUsed       uint64                `json:"gasUsed"`
	TotalScore    uint64                `json:"totalScore"`
	TxsRoot       thor.Bytes32          `json:"txsRoot"`
	TxsFeatures   uint32                `json:"txsFeatures"`
	StateRoot     thor.Bytes32          `json:"stateRoot"`
	ReceiptsRoot  thor.Bytes32          `json:"receiptsRoot"`
	COM           bool                  `json:"com"`
	Signer        thor.Address          `json:"signer"`
	IsTrunk       bool                  `json:"isTrunk"`
	IsFinalized   bool                  `json:"isFinalized"`
	BaseFeePerGas *math.HexOrDecimal256 `json:"baseFeePerGas,omitempty"`
}

type RawBlockSummary struct {
	Raw string `json:"raw"`
}

type CollapsedBlock struct {
	*BlockSummary
	Transactions []thor.Bytes32 `json:"transactions"`
}

type EmbeddedTx struct {
	ID                   thor.Bytes32          `json:"id"`
	Type                 uint8                 `json:"type"`
	ChainTag             byte                  `json:"chainTag"`
	BlockRef             string                `json:"blockRef"`
	Expiration           uint32                `json:"expiration"`
	Clauses              Clauses               `json:"clauses"`
	GasPriceCoef         *uint8                `json:"gasPriceCoef,omitempty"`
	MaxFeePerGas         *math.HexOrDecimal256 `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *math.HexOrDecimal256 `json:"maxPriorityFeePerGas,omitempty"`
	Gas                  uint64                `json:"gas"`
	Origin               thor.Address          `json:"origin"`
	Delegator            *thor.Address         `json:"delegator"`
	Nonce                math.HexOrDecimal64   `json:"nonce"`
	DependsOn            *thor.Bytes32         `json:"dependsOn"`
	Size                 uint32                `json:"size"`

	// receipt part
	GasUsed  uint64                `json:"gasUsed"`
	GasPayer thor.Address          `json:"gasPayer"`
	Paid     *math.HexOrDecimal256 `json:"paid"`
	Reward   *math.HexOrDecimal256 `json:"reward"`
	Reverted bool                  `json:"reverted"`
	Outputs  []*Output             `json:"outputs"`
}

type ExpandedBlock struct {
	*BlockSummary
	Transactions []*EmbeddedTx `json:"transactions"`
}
