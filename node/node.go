package node

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/node/blockpool"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type blockBuilder func(*state.Creator) (*block.Block, error)

// Options for Node.
type Options struct {
	DataPath    string
	Bind        string
	Proposer    thor.Address
	Beneficiary thor.Address
	PrivateKey  *ecdsa.PrivateKey
}

// Node is the abstraction of local node.
type Node struct {
	op              Options
	wg              *sync.WaitGroup
	genesisBuild    blockBuilder
	bp              *blockpool.BlockPool
	bestBlockUpdate chan bool
}

// New is a factory for Node.
func New(op Options) *Node {
	return &Node{
		op:              op,
		wg:              new(sync.WaitGroup),
		genesisBuild:    genesis.Mainnet.Build,
		bp:              blockpool.New(),
		bestBlockUpdate: make(chan bool, 2)}
}

// SetGenesisBuild 现在是专门为测试时方便选择 genesisBuild 的.
func (n *Node) SetGenesisBuild(genesisBuild blockBuilder) {
	n.genesisBuild = genesisBuild
}

// Run start node services and block func exit,
// until the parent context had been canceled.
func (n *Node) Run(ctx context.Context) error {
	stateC, chain, lv, lsr, err := n.prepare()
	if err != nil {
		return err
	}
	defer lv.Close()

	svr := service{
		ctx:   ctx,
		chain: chain}

	n.routine(func() {
		svr.withRestful(&http.Server{Handler: api.NewHTTPHandler(chain, stateC)}, lsr).run()
	})

	n.routine(func() {
		svr.withConsensus(consensus.New(chain, stateC), n.bestBlockUpdate, n.bp).run()
	})

	n.routine(func() {
		svr.withPacker(packer.New(n.op.Proposer, n.op.Beneficiary, chain, stateC), n.bestBlockUpdate, n.op.PrivateKey).run()
	})

	n.wg.Wait()
	return nil
}

func (n *Node) routine(f func()) {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		f()
	}()
}

func (n *Node) prepare() (*state.Creator, *chain.Chain, *lvldb.LevelDB, net.Listener, error) {
	if n.op.DataPath == "" {
		return nil, nil, nil, nil, errors.New("open batabase") // ephemeral
	}

	lv, err := lvldb.New(n.op.DataPath, lvldb.Options{})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	stateC := state.NewCreator(lv)

	genesisBlock, err := n.genesisBuild(stateC)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	chain := chain.New(lv)
	if err := chain.WriteGenesis(genesisBlock); err != nil {
		return nil, nil, nil, nil, err
	}

	lsr, err := net.Listen("tcp", n.op.Bind)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return stateC, chain, lv, lsr, nil
}
