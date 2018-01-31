package types

import (
	"github.com/vechain/thor/block"
)

//Block block
type Block struct {
	Number      uint32
	ID          string
	ParentID    string
	Timestamp   uint64
	TotalScore  uint64
	GasLimit    uint64
	GasUsed     uint64
	Beneficiary string

	TxsRoot      string
	StateRoot    string
	ReceiptsRoot string
	Txs          []string
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
