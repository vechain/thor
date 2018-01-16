package types

import (
	"github.com/vechain/thor/block"
)

//Block block
type Block struct {
	Number      uint32
	Hash        string
	ParentHash  string
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
	txhs := make([]string, len(txs))
	for i, tx := range txs {
		txhs[i] = tx.Hash().String()
	}

	header := b.Header()

	return &Block{
		Number:       b.Number(),
		Hash:         b.Hash().String(),
		ParentHash:   b.ParentHash().String(),
		Timestamp:    b.Timestamp(),
		TotalScore:   header.TotalScore(),
		GasLimit:     header.GasLimit(),
		GasUsed:      header.GasUsed(),
		Beneficiary:  header.Beneficiary().String(),
		StateRoot:    header.StateRoot().String(),
		ReceiptsRoot: header.ReceiptsRoot().String(),
		TxsRoot:      header.TxsRoot().String(),

		Txs: txhs,
	}
}
