package node

import (
	"context"
	"errors"
	"net"

	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type stateCreater func(thor.Hash) (*state.State, error)

// Options for Node.
type Options struct {
	dataPath string
	bind     string
}

// Node is the abstraction of local node.
type Node struct {
	op Options
}

// New is a factory for Node.
func New(op Options) *Node {
	return &Node{op}
}

// Run will start some block chain services and block func exit,
// until the parent context had been canceled.
func (n *Node) Run(ctx context.Context) error {
	lv, err := n.openDatabase()
	if err != nil {
		return err
	}
	defer lv.Close()

	stateC := func(hash thor.Hash) (*state.State, error) {
		return state.New(hash, lv)
	}

	genesisBlock, err := makeGenesisBlock(stateC, genesis.Build)
	if err != nil {
		return err
	}

	chain, err := makeChain(lv, genesisBlock)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", n.op.bind)
	if err != nil {
		return err
	}

	go restfulService(ctx, listener, chain, stateC)

	go consensusService(ctx)

	go proposerService(ctx)

	<-ctx.Done()
	return nil
}

func (n *Node) openDatabase() (*lvldb.LevelDB, error) {
	if n.op.dataPath == "" {
		return nil, errors.New("open batabase") // ephemeral
	}
	return lvldb.New(n.op.dataPath, lvldb.Options{})
}
