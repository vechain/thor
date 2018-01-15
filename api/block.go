package api

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/thor"
)

//Block block
type Block struct {
	Number      uint32
	Hash        thor.Hash
	ParentHash  thor.Hash
	Timestamp   uint64
	TotalScore  uint64
	GasLimit    uint64
	GasUsed     uint64
	Beneficiary thor.Address

	TxsRoot      thor.Hash
	StateRoot    thor.Hash
	ReceiptsRoot thor.Hash
	Txs          Transactions
}

//BlockInterface for manage block with chain
type BlockInterface struct {
	chain *chain.Chain
}

//NewBlockInterface return a BlockMananger by chain
func NewBlockInterface(chain *chain.Chain) *BlockInterface {
	return &BlockInterface{
		chain: chain,
	}
}

//GetBlockByHash return block by address
func (bi *BlockInterface) GetBlockByHash(blockHash thor.Hash) (*Block, error) {
	b, err := bi.chain.GetBlock(blockHash)
	if err != nil {
		return nil, err
	}
	return convertBlock(b), nil
}

//GetBlockByNumber return block by address
func (bi *BlockInterface) GetBlockByNumber(blockNumber uint32) (*Block, error) {
	b, err := bi.chain.GetBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}
	return convertBlock(b), nil
}

func convertBlock(b *block.Block) *Block {
	txs := b.Transactions()
	txsCopy := make(Transactions, len(txs))
	singing := cry.NewSigning(b.Hash())
	for i, tx := range txs {
		txsCopy[i] = convertTransactionWithSigning(tx, singing)
	}
	header := b.Header()
	return &Block{
		Number:       b.Number(),
		Hash:         b.Hash(),
		ParentHash:   b.ParentHash(),
		Timestamp:    b.Timestamp(),
		TotalScore:   header.TotalScore(),
		GasLimit:     header.GasLimit(),
		GasUsed:      header.GasUsed(),
		Beneficiary:  header.Beneficiary(),
		StateRoot:    header.StateRoot(),
		ReceiptsRoot: header.ReceiptsRoot(),
		TxsRoot:      header.TxsRoot(),

		Txs: txsCopy,
	}
}
