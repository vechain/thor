package node

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type stateCreater func(thor.Hash) (*state.State, error)

// Options for Node.
type Options struct {
	DataPath string
	Bind     string
}

// Node is the abstraction of local node.
type Node struct {
	op Options
	wg *sync.WaitGroup
}

// New is a factory for Node.
func New(op Options) *Node {
	return &Node{
		op: op,
		wg: new(sync.WaitGroup)}
}

// Run will start some block chain services and block func exit,
// until the parent context had been canceled.
func (n *Node) Run(ctx context.Context) error {
	lv, err := n.openDatabase()
	if err != nil {
		return err
	}
	defer lv.Close()

	stateC := state.NewCreator(lv).NewState

	genesisBlock, err := makeGenesisBlock(stateC, genesis.Build)
	if err != nil {
		return err
	}

	chain, err := makeChain(lv, genesisBlock)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", n.op.Bind)
	if err != nil {
		return err
	}

	bp := newBlockPool()

	routine := func(f func()) {
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			f()
		}()
	}

	routine(func() { restfulService(ctx, listener, chain, stateC) })
	routine(func() { consensusService(ctx, bp, chain, stateC) })
	routine(func() { proposerService(ctx, bp) })

	n.wg.Wait()
	return nil
}

func (n *Node) openDatabase() (*lvldb.LevelDB, error) {
	if n.op.DataPath == "" {
		return nil, errors.New("open batabase") // ephemeral
	}
	return lvldb.New(n.op.DataPath, lvldb.Options{})
}
