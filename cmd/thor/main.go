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

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	Genesis "github.com/vechain/thor/genesis"
	Logdb "github.com/vechain/thor/logdb"
	Lvldb "github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	version   = "1.0"
	gitCommit string
	release   = "dev"

	log = log15.New()
)

var boot = "enode://b788e1d863aaea4fecef4aba4be50e59344d64f2db002160309a415ab508977b8bffb7bac3364728f9cdeab00ebdd30e8d02648371faacd0819edc27c18b2aad@106.15.4.191:55555"

// Options for Client.
type Options struct {
	DataPath    string
	Bind        string
	Proposer    thor.Address
	Beneficiary thor.Address
	PrivateKey  *ecdsa.PrivateKey
}

func main() {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Thor"
	app.Usage = "Core of VeChain"
	app.Copyright = "2018 VeChain Foundation <https://vechain.org/>"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "p2paddr",
			Value: ":11235",
			Usage: "p2p listen addr",
		},
		cli.StringFlag{
			Name:  "apiaddr",
			Value: "127.0.0.1:8669",
			Usage: "restful addr",
		},
		// cli.StringFlag{
		// 	Name:  "nodekey",
		// 	Usage: "private key (for node) file path (defaults to ~/.thor-node.key if omitted)",
		// },
		cli.StringFlag{
			Name:  "key",
			Usage: "private key (for pack) as hex (for testing)",
		},
		cli.StringFlag{
			Name:  "datadir",
			Value: "/tmp/thor-data",
			Usage: "chain data path",
		},
		cli.IntFlag{
			Name:  "verbosity",
			Value: int(log15.LvlInfo),
			Usage: "log verbosity (0-9)",
		},
		cli.BoolFlag{
			Name:  "devnet",
			Usage: "develop network",
		},
	}
	app.Action = action

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func action(ctx *cli.Context) error {
	initLog(log15.Lvl(ctx.Int("verbosity")))

	var (
		genesis *Genesis.Genesis
		err     error
	)
	if ctx.Bool("devnet") {
		genesis, err = Genesis.NewDevnet()
	} else {
		genesis, err = Genesis.NewMainnet()
	}
	if err != nil {
		return err
	}
	dataDir := fmt.Sprintf("%v/chain-%x", ctx.String("datadir"), genesis.ID().Bytes()[24:])
	if err = os.MkdirAll(dataDir, os.ModePerm); err != nil {
		if !os.IsExist(err) {
			return err
		}
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

	stateCreator := state.NewCreator(lvldb)

	genesisBlock, txLogs, err := genesis.Build(stateCreator)
	if err != nil {
		return err
	}

	logs := []*Logdb.Log{}
	for _, log := range txLogs {
		logs = append(logs, Logdb.NewLog(genesisBlock.Header(), 0, thor.Bytes32{}, thor.Address{}, log))
	}
	logdb.Insert(logs, nil)

	chain, err := chain.New(lvldb, genesisBlock)
	if err != nil {
		return err
	}

	nodeKey, err := loadNodeKey(dataDir + "/node-key")
	if err != nil {
		return err
	}

	proposer, privateKey, err := loadAccount(ctx.String("key"))
	if err != nil {
		return err
	}

	lsr, err := net.Listen("tcp", ctx.String("apiaddr"))
	if err != nil {
		return err
	}
	defer lsr.Close()

	txpool := txpool.New(chain, stateCreator)
	communicator := comm.New(chain, txpool)
	consensus := consensus.New(chain, stateCreator)
	packer := packer.New(chain, stateCreator, proposer, proposer)
	rest := &http.Server{Handler: api.New(chain, stateCreator, txpool, logdb)}
	opt := &p2psrv.Options{
		PrivateKey:     nodeKey,
		MaxPeers:       25,
		ListenAddr:     ctx.String("p2paddr"),
		BootstrapNodes: []*discover.Node{discover.MustParseNode(boot)},
	}

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())

	goes.Go(func() {
		runCommunicator(c, communicator, opt, dataDir+"/good-nodes")
		log.Info("communicator exit")
	})

	txRoutineCtx := &txRoutineContext{
		ctx:          c,
		communicator: communicator,
		txpool:       txpool,
	}
	goes.Go(func() {
		txBroadcastLoop(txRoutineCtx)
		log.Info("broadcast loop exit")
	})
	goes.Go(func() {
		txPoolUpdateLoop(txRoutineCtx)
		log.Info("tx pool update loop exit")
	})

	blockRoutineCtx := &blockRoutineContext{
		ctx:              c,
		communicator:     communicator,
		chain:            chain,
		txpool:           txpool,
		packedChan:       make(chan *packedEvent),
		bestBlockUpdated: make(chan *block.Block, 1),
	}
	goes.Go(func() {
		consentLoop(blockRoutineCtx, consensus, logdb)
		log.Info("consent loop exit")
	})
	goes.Go(func() {
		packLoop(blockRoutineCtx, packer, privateKey)
		log.Info("pack loop exit")
	})

	goes.Go(func() {
		runRestful(c, rest, lsr)
		log.Info("restful exit")
	})

	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	select {
	case <-interrupt:
		go func() {
			// force exit when rcvd 10 interrupts
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
	if err := p2pSrv.Start(communicator.Protocols()); err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
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

	communicator.Start(peerCh)
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
