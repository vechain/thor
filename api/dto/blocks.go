// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/thor"
)

type BlockBase struct {
	Number        uint32                `json:"number"`
	ID            thor.Bytes32          `json:"id"`
	Size          uint32                `json:"size"`
	ParentID      thor.Bytes32          `json:"parentID"`
	Timestamp     uint64                `json:"timestamp"`
	GasLimit      uint64                `json:"gasLimit"`
	Beneficiary   thor.Address          `json:"beneficiary"`
	GasUsed       uint64                `json:"gasUsed"`
	BaseFeePerGas *math.HexOrDecimal256 `json:"baseFeePerGas,omitempty"`
	TotalScore    uint64                `json:"totalScore"`
	TxsRoot       thor.Bytes32          `json:"txsRoot"`
	TxsFeatures   uint32                `json:"txsFeatures"`
	StateRoot     thor.Bytes32          `json:"stateRoot"`
	ReceiptsRoot  thor.Bytes32          `json:"receiptsRoot"`
	COM           bool                  `json:"com"`
	Signer        thor.Address          `json:"signer"`
}

type BlockSummary struct {
	BlockBase
	IsTrunk     bool `json:"isTrunk"`
	IsFinalized bool `json:"isFinalized"`
}

type RawBlockSummary struct {
	Raw string `json:"raw"`
}

type CollapsedBlock struct {
	*BlockSummary
	Transactions []thor.Bytes32 `json:"transactions"`
}

type EmbeddedTx struct {
	TransactionBase

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
