package types

import (
	"github.com/vechain/thor/block"
)

//Block block
type Block struct {
	Number      uint32 `json:"number"`
	ID          string `json:"id"`
	ParentID    string `json:"parentID"`
	Timestamp   uint64 `json:"timestamp,string"`
	TotalScore  uint64 `json:"totalScore,string"`
	GasLimit    uint64 `json:"gasLimit,string"`
	GasUsed     uint64 `json:"gasUsed,string"`
	Beneficiary string `json:"beneficiary"`

	TxsRoot      string   `json:"txsRoot"`
	StateRoot    string   `json:"stateRoot"`
	ReceiptsRoot string   `json:"receiptsRoot"`
	Txs          []string `json:"txs,string"`
}

//ConvertBlock convert a raw block into a json format block
func ConvertBlock(b *block.Block) *Block {

	txs := b.Transactions()
	txIds := make([]string, len(txs))
	for i, tx := range txs {
		txIds[i] = tx.ID().String()
	}

	header := b.Header()

	return &Block{
		Number:       header.Number(),
		ID:           header.ID().String(),
		ParentID:     header.ParentID().String(),
		Timestamp:    header.Timestamp(),
		TotalScore:   header.TotalScore(),
		GasLimit:     header.GasLimit(),
		GasUsed:      header.GasUsed(),
		Beneficiary:  header.Beneficiary().String(),
		StateRoot:    header.StateRoot().String(),
		ReceiptsRoot: header.ReceiptsRoot().String(),
		TxsRoot:      header.TxsRoot().String(),

		Txs: txIds,
	}
}
