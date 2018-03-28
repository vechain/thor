package blocks

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

//Block block
type Block struct {
	Number      uint32              `json:"number"`
	ID          thor.Hash           `json:"id"`
	ParentID    thor.Hash           `json:"parentID"`
	Timestamp   math.HexOrDecimal64 `json:"timestamp"`
	TotalScore  math.HexOrDecimal64 `json:"totalScore"`
	GasLimit    math.HexOrDecimal64 `json:"gasLimit"`
	GasUsed     math.HexOrDecimal64 `json:"gasUsed"`
	Beneficiary thor.Address        `json:"beneficiary"`

	TxsRoot      thor.Hash   `json:"txsRoot"`
	StateRoot    thor.Hash   `json:"stateRoot"`
	ReceiptsRoot thor.Hash   `json:"receiptsRoot"`
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
		Timestamp:    math.HexOrDecimal64(header.Timestamp()),
		TotalScore:   math.HexOrDecimal64(header.TotalScore()),
		GasLimit:     math.HexOrDecimal64(header.GasLimit()),
		GasUsed:      math.HexOrDecimal64(header.GasUsed()),
		Beneficiary:  header.Beneficiary(),
		StateRoot:    header.StateRoot(),
		ReceiptsRoot: header.ReceiptsRoot(),
		TxsRoot:      header.TxsRoot(),

		Txs: txIds,
	}
}
