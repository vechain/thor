package api

import (
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

type blockGetter interface {
	GetBlockByNumber(uint32) (*block.Block, error)
	GetBlock(thor.Hash) (*block.Block, error)
}

//BlockInterface for manage block with chain
type BlockInterface struct {
	blkGetter blockGetter
}

//NewBlockInterface return a BlockMananger by chain
func NewBlockInterface(blkGetter blockGetter) *BlockInterface {
	return &BlockInterface{
		blkGetter: blkGetter,
	}

}

//GetBlockByHash return block by address
func (bi *BlockInterface) GetBlockByHash(blockHash thor.Hash) (*types.Block, error) {
	b, err := bi.blkGetter.GetBlock(blockHash)
	if err != nil {
		return nil, err
	}

	return types.ConvertBlock(b), nil
}

//GetBlockByNumber return block by address
func (bi *BlockInterface) GetBlockByNumber(blockNumber uint32) (*types.Block, error) {
	b, err := bi.blkGetter.GetBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}

	return types.ConvertBlock(b), nil
}
