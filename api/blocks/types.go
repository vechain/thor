// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"github.com/vechain/thor/block"
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
	StateRoot    thor.Bytes32   `json:"stateRoot"`
	ReceiptsRoot thor.Bytes32   `json:"receiptsRoot"`
	Signer       thor.Address   `json:"signer"`
	IsTrunk      bool           `json:"isTrunk"`
	Transactions []thor.Bytes32 `json:"transactions,string"`
}

func convertBlock(b *block.Block, isTrunk bool) (*Block, error) {
	if b == nil {
		return nil, nil
	}
	signer, err := b.Header().Signer()
	if err != nil {
		return nil, err
	}
	txs := b.Transactions()
	txIds := make([]thor.Bytes32, len(txs))
	for i, tx := range txs {
		txIds[i] = tx.ID()
	}

	header := b.Header()
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
		Size:         uint32(b.Size()),
		StateRoot:    header.StateRoot(),
		ReceiptsRoot: header.ReceiptsRoot(),
		TxsRoot:      header.TxsRoot(),
		IsTrunk:      isTrunk,
		Transactions: txIds,
	}, nil
}
