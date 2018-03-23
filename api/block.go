package api

import (
	"github.com/vechain/thor/api/types"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

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

//GetBlockByID return block by address
func (bi *BlockInterface) GetBlockByID(blockID thor.Hash) (*types.Block, error) {
	b, err := bi.chain.GetBlock(blockID)
	if err != nil {
		if bi.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return types.ConvertBlock(b), nil
}

//GetBlockByNumber return block by address
func (bi *BlockInterface) GetBlockByNumber(blockNumber uint32) (*types.Block, error) {
	b, err := bi.chain.GetBlockByNumber(blockNumber)
	if err != nil {
		if bi.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return types.ConvertBlock(b), nil
}

//GetBestBlock returns latest block
func (bi *BlockInterface) GetBestBlock() (*types.Block, error) {
	b, err := bi.chain.GetBestBlock()
	if err != nil {
		return nil, err
	}

	return types.ConvertBlock(b), nil
}
