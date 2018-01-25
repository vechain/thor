package node

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type blockBuilder func(*state.State) (*block.Block, error)

func makeGenesisBlock(stateC stateCreater, blockB blockBuilder) (*block.Block, error) {
	state, err := stateC(thor.Hash{})
	if err != nil {
		return nil, err
	}
	return blockB(state)
}

func makeChain(lv *lvldb.LevelDB, genesisBlock *block.Block) (*chain.Chain, error) {
	chain := chain.New(lv)
	if err := chain.WriteGenesis(genesisBlock); err != nil {
		return nil, err
	}
	return chain, nil
}
