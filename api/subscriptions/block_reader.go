package subscriptions

import (
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type BlockReader struct {
	chain       *chain.Chain
	blockReader chain.BlockReader
}

func NewBlockReader(chain *chain.Chain, fromBlock thor.Bytes32) *BlockReader {
	return &BlockReader{
		chain:       chain,
		blockReader: chain.NewBlockReader(fromBlock),
	}
}

func (br *BlockReader) Read() ([]interface{}, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, err
	}
	result := []interface{}{}
	for _, b := range blocks {
		block, err := convertBlock(b)
		if err != nil {
			return nil, err
		}
		result = append(result, block)
	}
	return result, nil
}
