package api

import (
	"github.com/vechain/thor/block"
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

//GetBlockByHash return block by address
func (bi *BlockInterface) GetBlockByHash(blockHash thor.Hash) (*block.Block, error) {
	return bi.chain.GetBlock(blockHash)
}

//GetBlockByNumber return block by address
func (bi *BlockInterface) GetBlockByNumber(blockNumber uint32) (*block.Block, error) {
	return bi.chain.GetBlockByNumber(blockNumber)
}
