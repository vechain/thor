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

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	Logdb "github.com/vechain/thor/logdb"
	Lvldb "github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/tx"
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

type components struct {
	chain        *chain.Chain
	txpool       *txpool.TxPool
	p2p          *p2psrv.Server
	communicator *comm.Communicator
	consensus    *consensus.Consensus
	packer       *packer.Packer
	rest         *http.Server
}

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

	proposer, privateKey, err := loadProposer(ctx.Bool("devnet"), dataDir+"/master.key")
	if err != nil {
		return err
	}
	log.Info("Proposer key loaded", "address", proposer)

	components, err := makeComponent(ctx, lvldb, logdb, genesis, proposer, dataDir)
	if err != nil {
		return err
	}

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())

	goes.Go(func() { runNetwork(c, components, dataDir) })
	goes.Go(func() { runRestful(c, components.rest, ctx.String("apiaddr")) })
	goes.Go(func() { synchronizeTx(c, components) })
	goes.Go(func() { produceBlock(c, components, logdb, privateKey) })

	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	select {
	case <-interrupt:
		log.Warn("Got sigterm, shutting down...")
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

func runNetwork(ctx context.Context, components *components, dataDir string) {
	protocols := components.communicator.Protocols()
	if err := components.p2p.Start(protocols); err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}

	for _, protocol := range protocols {
		log.Info("P2P Protocol used", "name", protocol.Name, "version", protocol.Version, "disc-topic", protocol.DiscTopic)
	}

	defer func() {
		components.p2p.Stop()
		nodes := components.p2p.GoodNodes()
		data, err := rlp.EncodeToBytes(nodes)
		if err != nil {
			log.Error(fmt.Sprintf("%v", err))
			return
		}
		if err := ioutil.WriteFile(dataDir+"/nodes.cache", data, 0644); err != nil {
			log.Error(fmt.Sprintf("%v", err))
		}
	}()

	peerCh := make(chan *p2psrv.Peer)
	components.p2p.SubscribePeer(peerCh)

	syncReport := func(count int, size metric.StorageSize, elapsed time.Duration) {
		log.Info("Block synchronized", "count", count, "size", size, "elapsed", elapsed.String())
	}
	components.communicator.Start(peerCh, syncReport)
	defer components.communicator.Stop()

	<-ctx.Done()
	log.Info("Communicator stoped")
}

// func runCommunicator(ctx context.Context, communicator *comm.Communicator, opt *p2psrv.Options, filePath string) {
// 	var nodes p2psrv.Nodes
// 	nodesByte, err := ioutil.ReadFile(filePath)
// 	if err != nil {
// 		if !os.IsNotExist(err) {
// 			log.Error(fmt.Sprintf("%v", err))
// 		}
// 	} else {
// 		rlp.DecodeBytes(nodesByte, &nodes)
// 		opt.GoodNodes = nodes
// 	}

// 	p2pSrv := p2psrv.New(opt)
// 	protocols := communicator.Protocols()
// 	if err := p2pSrv.Start(protocols); err != nil {
// 		log.Error(fmt.Sprintf("%v", err))
// 		return
// 	}

// 	defer func() {
// 		p2pSrv.Stop()
// 		nodes := p2pSrv.GoodNodes()
// 		data, err := rlp.EncodeToBytes(nodes)
// 		if err != nil {
// 			log.Error(fmt.Sprintf("%v", err))
// 			return
// 		}
// 		if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
// 			log.Error(fmt.Sprintf("%v", err))
// 		}
// 	}()

// 	peerCh := make(chan *p2psrv.Peer)
// 	p2pSrv.SubscribePeer(peerCh)

// 	syncReport := func(count int, size metric.StorageSize, elapsed time.Duration) {
// 		log.Info("Block synchronized", "count", count, "size", size, "elapsed", elapsed.String())
// 	}
// 	communicator.Start(peerCh, syncReport)
// 	defer communicator.Stop()

// 	log.Info("Communicator started", "listen-addr", opt.ListenAddr, "max-peers", opt.MaxPeers)
// 	for _, protocol := range protocols {
// 		log.Info("Protocol parsed", "name", protocol.Name, "version", protocol.Version, "disc-topic", protocol.DiscTopic)
// 	}

// 	<-ctx.Done()
// 	log.Info("Communicator stoped")
// }

func runRestful(ctx context.Context, rest *http.Server, apiAddr string) {
	lsr, err := net.Listen("tcp", apiAddr)
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}
	defer lsr.Close()

	go func() {
		<-ctx.Done()
		rest.Shutdown(context.TODO())
	}()

	log.Info("Rest service started", "listen-addr", apiAddr)
	if err := rest.Serve(lsr); err != http.ErrServerClosed {
		log.Error(fmt.Sprintf("%v", err))
	}
	log.Info("Rest service stoped")
}

func synchronizeTx(ctx context.Context, components *components) {
	var goes co.Goes

	// routine for broadcast new tx
	goes.Go(func() {
		txCh := make(chan *tx.Transaction)
		sub := components.txpool.SubscribeNewTransaction(txCh)

		for {
			select {
			case <-ctx.Done():
				sub.Unsubscribe()
				return
			case tx := <-txCh:
				components.communicator.BroadcastTx(tx)
			}
		}
	})

	// routine for update txpool
	goes.Go(func() {
		txCh := make(chan *tx.Transaction)
		sub := components.communicator.SubscribeTx(txCh)

		for {
			select {
			case <-ctx.Done():
				sub.Unsubscribe()
				return
			case tx := <-txCh:
				components.txpool.Add(tx)
			}
		}
	})

	log.Info("Transaction synchronization started")
	goes.Wait()
	log.Info("Transaction synchronization stoped")
}
