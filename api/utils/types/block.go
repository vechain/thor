package types

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

//Block block
type Block struct {
	Number      uint32       `json:"number"`
	ID          thor.Hash    `json:"id,string"`
	ParentID    thor.Hash    `json:"parentID,string"`
	Timestamp   uint64       `json:"timestamp,string"`
	TotalScore  uint64       `json:"totalScore,string"`
	GasLimit    uint64       `json:"gasLimit,string"`
	GasUsed     uint64       `json:"gasUsed,string"`
	Beneficiary thor.Address `json:"beneficiary,string"`

	TxsRoot      thor.Hash   `json:"txsRoot,string"`
	StateRoot    thor.Hash   `json:"stateRoot,string"`
	ReceiptsRoot thor.Hash   `json:"receiptsRoot,string"`
	Txs          []thor.Hash `json:"txs,string"`
}

//ConvertBlock convert a raw block into a json format block
func ConvertBlock(b *block.Block) *Block {

	txs := b.Transactions()
	txIds := make([]thor.Hash, len(txs))
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
		StateRoot:    header.StateRoot(),
		ReceiptsRoot: header.ReceiptsRoot(),
		TxsRoot:      header.TxsRoot(),

		Txs: txIds,
	}
}
