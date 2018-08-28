package subscriptions

import (
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type BlockReader struct {
	chain       *chain.Chain
	blockReader chain.BlockReader
}

func NewBlockReader(chain *chain.Chain, position thor.Bytes32) *BlockReader {
	return &BlockReader{
		chain:       chain,
		blockReader: chain.NewBlockReader(position),
	}
}

func (br *BlockReader) Read() ([]interface{}, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []interface{}
	for _, block := range blocks {
		msg, err := convertBlock(block)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, len(blocks) > 0, nil
}
