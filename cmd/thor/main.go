package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
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
	gitCommit string
	version   = "1.0"
	release   = "dev"
	log       = log15.New()
	boot      = "enode://b788e1d863aaea4fecef4aba4be50e59344d64f2db002160309a415ab508977b8bffb7bac3364728f9cdeab00ebdd30e8d02648371faacd0819edc27c18b2aad@106.15.4.191:55555"
)

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

	var (
		genesis *Genesis.Genesis
		err     error
	)
	if ctx.Bool("devnet") {
		genesis, err = Genesis.NewDevnet()
		log.Info("Using Devnet", "genesis", genesis.ID().AbbrevString())
	} else {
		genesis, err = Genesis.NewMainnet()
		log.Info("Using Mainnet", "genesis", genesis.ID().AbbrevString())
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
	log.Info("Disk storage enabled for storing data", "path", dataDir)

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

	nodeKey, err := loadKey(dataDir + "/node.key")
	if err != nil {
		return err
	}
	log.Info("Node key loaded", "address", crypto.PubkeyToAddress(nodeKey.PublicKey))

	proposer, privateKey, err := loadProposer(ctx.Bool("devnet"), dataDir+"/master.key")
	if err != nil {
		return err
	}
	log.Info("Proposer key loaded", "address", proposer)

	beneficiary := proposer
	if ctx.String("beneficiary") != "" {
		if beneficiary, err = thor.ParseAddress(ctx.String("beneficiary")); err != nil {
			return err
		}
	}
	log.Info("Beneficiary key loaded", "address", beneficiary)

	restAddr := ctx.String("apiaddr")
	lsr, err := net.Listen("tcp", restAddr)
	if err != nil {
		return err
	}
	defer lsr.Close()

	txpool := txpool.New(chain, stateCreator)
	communicator := comm.New(chain, txpool)
	consensus := consensus.New(chain, stateCreator)
	packer := packer.New(chain, stateCreator, proposer, beneficiary)
	rest := &http.Server{Handler: api.New(chain, stateCreator, txpool, logdb)}
	opt := &p2psrv.Options{
		PrivateKey:     nodeKey,
		MaxPeers:       ctx.Int("maxpeers"),
		ListenAddr:     ctx.String("p2paddr"),
		BootstrapNodes: []*discover.Node{discover.MustParseNode(boot)},
	}

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())

	goes.Go(func() {
		timer := time.NewTimer(1 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-c.Done():
				return
			case <-timer.C:
				best, err := chain.GetBestBlock()
				if err != nil {
					log.Error(fmt.Sprintf("%v", err))
				} else {
					header := best.Header()
					signerStr := "N/A"
					if signer, err := header.Signer(); err == nil {
						signerStr = signer.String()
					}

					log.Info("Current best block",
						"number", header.Number(),
						"id", header.ID().AbbrevString(),
						"total-score", header.TotalScore(),
						"proposer", signerStr,
					)
				}

				if !communicator.IsSynced() {
					log.Warn("Chain data has not synced")
				}

				timer.Reset(15 * time.Second)
			}
		}
	})

	goes.Go(func() {
		runCommunicator(c, communicator, opt, dataDir+"/nodes.cache")
		log.Info("Communicator stoped")
	})

	goes.Go(func() {
		synchronizeTx(
			&txRoutineContext{
				ctx:          c,
				communicator: communicator,
				txpool:       txpool,
			},
		)
	})

	goes.Go(func() {
		produceBlock(
			&blockRoutineContext{
				ctx:              c,
				communicator:     communicator,
				chain:            chain,
				txpool:           txpool,
				packedChan:       make(chan *packedEvent),
				bestBlockUpdated: make(chan *block.Block, 1),
			},
			consensus,
			packer,
			privateKey,
			logdb,
		)
	})

	goes.Go(func() {
		log.Info("Rest service started", "listen-addr", restAddr)
		runRestful(c, rest, lsr)
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

	communicator.Start(peerCh)
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
