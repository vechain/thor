// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

//Block block
type Block struct {
	Number       uint32         `json:"number"`
	ID           thor.Bytes32   `json:"id"`
	Size         uint32         `json:"size"`
	ParentID     thor.Bytes32   `json:"parentID"`
	Timestamp    uint64         `json:"timestamp"`
	GasLimit     uint64         `json:"gasLimit"`
	Beneficiary  thor.Address   `json:"beneficiary"`
	GasUsed      uint64         `json:"gasUsed"`
	TotalScore   uint64         `json:"totalScore"`
	TxsRoot      thor.Bytes32   `json:"txsRoot"`
	TxsFeatures  uint32         `json:"txsFeatures"`
	StateRoot    thor.Bytes32   `json:"stateRoot"`
	ReceiptsRoot thor.Bytes32   `json:"receiptsRoot"`
	Signer       thor.Address   `json:"signer"`
	IsTrunk      bool           `json:"isTrunk"`
	Transactions []thor.Bytes32 `json:"transactions"`
}

func convertBlock(summary *chain.BlockSummary, isTrunk bool) (*Block, error) {
	header := summary.Header

	signer, err := header.Signer()
	if err != nil {
		return nil, err
	}

	return &Block{
		Number:       header.Number(),
		ID:           header.ID(),
		ParentID:     header.ParentID(),
		Timestamp:    header.Timestamp(),
		TotalScore:   header.TotalScore(),
		GasLimit:     header.GasLimit(),
		GasUsed:      header.GasUsed(),
		Beneficiary:  header.Beneficiary(),
		Signer:       signer,
		Size:         uint32(summary.Size),
		StateRoot:    header.StateRoot(),
		ReceiptsRoot: header.ReceiptsRoot(),
		TxsRoot:      header.TxsRoot(),
		TxsFeatures:  uint32(header.TxsFeatures()),
		IsTrunk:      isTrunk,
		Transactions: summary.Txs,
	}, nil
}
