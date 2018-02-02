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
	"github.com/vechain/thor/node/network"
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
	IP          string // 暂时为了区分本地 node
}

// Node is the abstraction of local node.
type Node struct {
	op           Options
	genesisBuild blockBuilder
}

// New is a factory for Node.
func New(op Options) *Node {
	return &Node{
		op:           op,
		genesisBuild: genesis.Mainnet.Build}
}

// SetGenesisBuild 现在是专门为测试时方便选择 genesisBuild 的.
func (n *Node) SetGenesisBuild(genesisBuild blockBuilder) {
	n.genesisBuild = genesisBuild
}

// Run start node services and block func exit,
// until the parent context had been canceled.
func (n *Node) Run(ctx context.Context, nw *network.Network) error {
	stateC, chain, lv, lsr, err := n.prepare()
	if err != nil {
		return err
	}
	defer lv.Close()

	bestBlockUpdate := make(chan bool, 2)
	defer close(bestBlockUpdate)

	wg, svr, routine := n.routineService(chain, stateC, nw, bestBlockUpdate)
	defer wg.Wait()

	routine(func() {
		svr.restful(&http.Server{Handler: api.NewHTTPHandler(chain, stateC)}, lsr).run(ctx)
	})
	routine(func() {
		svr.consensus(consensus.New(chain, stateC)).run(ctx)
	})
	routine(func() {
		svr.packer(packer.New(n.op.Proposer, n.op.Beneficiary, chain, stateC), n.op.PrivateKey).run(ctx)
	})

	return nil
}

func (n *Node) routineService(
	chain *chain.Chain,
	stateC *state.Creator,
	nw *network.Network,
	bestBlockUpdate chan bool) (wg *sync.WaitGroup, svr *service, routine func(func())) {
	wg = &sync.WaitGroup{}
	svr = newService(chain, stateC, nw, n.op.IP, bestBlockUpdate)
	routine = func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	return
}

func (n *Node) prepare() (*state.Creator, *chain.Chain, *lvldb.LevelDB, net.Listener, error) {
	if n.op.DataPath == "" {
		return nil, nil, nil, nil, errors.New("open batabase")
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
