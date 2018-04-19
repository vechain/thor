package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	Logdb "github.com/vechain/thor/logdb"
	Lvldb "github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	gitCommit string
	version   = "1.0"
	release   = "dev"
	log       = log15.New()
	boot      = "enode://b788e1d863aaea4fecef4aba4be50e59344d64f2db002160309a415ab508977b8bffb7bac3364728f9cdeab00ebdd30e8d02648371faacd0819edc27c18b2aad@106.15.4.191:55555"
)

type component struct {
	chain        *chain.Chain
	txpool       *txpool.TxPool
	communicator *comm.Communicator
	nodeKey      *ecdsa.PrivateKey
	consensus    *consensus.Consensus
	packer       *packer.Packer
	privateKey   *ecdsa.PrivateKey
	rest         *http.Server
}

// txpool := txpool.New(chain, stateCreator)
// communicator := comm.New(chain, txpool)
// consensus := consensus.New(chain, stateCreator)
// packer := packer.New(chain, stateCreator, proposer, beneficiary)
// rest := &http.Server{Handler: api.New(chain, stateCreator, txpool, logdb, communicator)}

func main() {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Thor"
	app.Usage = "Core of VeChain"
	app.Copyright = "2018 VeChain Foundation <https://vechain.org/>"
	app.Flags = appFlags
	app.Action = action

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func action(ctx *cli.Context) error {
	initLog(log15.Lvl(ctx.Int("verbosity")))
	log.Info("Welcome to thor network")

	genesis, err := genesis(ctx.Bool("devnet"))
	if err != nil {
		return err
	}

	dataDir, err := dataDir(genesis, ctx.String("datadir"))
	if err != nil {
		return err
	}

	lvldb, err := Lvldb.New(dataDir+"/main.db", Lvldb.Options{})
	if err != nil {
		return err
	}
	defer lvldb.Close()

	logdb, err := Logdb.New(dataDir + "/log.db")
	if err != nil {
		return err
	}
	defer logdb.Close()

	restAddr := ctx.String("apiaddr")
	lsr, err := net.Listen("tcp", restAddr)
	if err != nil {
		return err
	}
	defer lsr.Close()

	component, err := makeComponent(lvldb, logdb, genesis, dataDir, ctx)

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())

	goes.Go(func() {
		runCommunicator(
			c,
			component.communicator,
			&p2psrv.Options{
				PrivateKey:     component.nodeKey,
				MaxPeers:       ctx.Int("maxpeers"),
				ListenAddr:     ctx.String("p2paddr"),
				BootstrapNodes: []*discover.Node{discover.MustParseNode(boot)},
			},
			dataDir+"/nodes.cache",
		)
		log.Info("Communicator stoped")
	})

	goes.Go(func() {
		synchronizeTx(
			&txRoutineContext{
				ctx:          c,
				communicator: component.communicator,
				txpool:       component.txpool,
			},
		)
	})

	goes.Go(func() {
		produceBlock(
			&blockRoutineContext{
				ctx:              c,
				communicator:     component.communicator,
				chain:            component.chain,
				txpool:           component.txpool,
				packedChan:       make(chan *packedEvent),
				bestBlockUpdated: make(chan *block.Block, 1),
			},
			component.consensus,
			component.packer,
			component.privateKey,
			logdb,
		)
	})

	goes.Go(func() {
		log.Info("Rest service started", "listen-addr", restAddr)
		runRestful(c, component.rest, lsr)
		log.Info("Rest service stoped")
	})

	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	select {
	case <-interrupt:
		log.Info("Got sigterm, shutting down...")
		go func() {
			// force exited when rcvd 10 interrupts
			for i := 0; i < 10; i++ {
				<-interrupt
			}
			os.Exit(1)
		}()
		cancel()
		goes.Wait()
	}

	return nil
}

func runCommunicator(ctx context.Context, communicator *comm.Communicator, opt *p2psrv.Options, filePath string) {
	var nodes p2psrv.Nodes
	nodesByte, err := ioutil.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Error(fmt.Sprintf("%v", err))
		}
	} else {
		rlp.DecodeBytes(nodesByte, &nodes)
		opt.GoodNodes = nodes
	}

	p2pSrv := p2psrv.New(opt)
	protocols := communicator.Protocols()
	if err := p2pSrv.Start(protocols); err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}
	for _, protocol := range protocols {
		log.Info("Protocol parsed", "name", protocol.Name, "version", protocol.Version, "disc-topic", protocol.DiscTopic)
	}

	defer func() {
		p2pSrv.Stop()
		nodes := p2pSrv.GoodNodes()
		data, err := rlp.EncodeToBytes(nodes)
		if err != nil {
			log.Error(fmt.Sprintf("%v", err))
			return
		}
		if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
			log.Error(fmt.Sprintf("%v", err))
		}
	}()

	peerCh := make(chan *p2psrv.Peer)
	p2pSrv.SubscribePeer(peerCh)

	syncReport := func(count int, size metric.StorageSize, elapsed time.Duration) {
		log.Info("Block synchronized", "count", count, "size", size, "elapsed", elapsed.String())
	}
	communicator.Start(peerCh, syncReport)
	log.Info("Communicator started", "listen-addr", opt.ListenAddr, "max-peers", opt.MaxPeers)
	defer communicator.Stop()

	<-ctx.Done()
}

func runRestful(ctx context.Context, rest *http.Server, lsr net.Listener) {
	go func() {
		<-ctx.Done()
		rest.Shutdown(context.TODO())
	}()

	if err := rest.Serve(lsr); err != http.ErrServerClosed {
		log.Error(fmt.Sprintf("%v", err))
	}
}
