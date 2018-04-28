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

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/p2psrv"
	Packer "github.com/vechain/thor/packer"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	version   string
	gitCommit string
	release   = "dev"
	log       = log15.New()
	boot      = "enode://b788e1d863aaea4fecef4aba4be50e59344d64f2db002160309a415ab508977b8bffb7bac3364728f9cdeab00ebdd30e8d02648371faacd0819edc27c18b2aad@106.15.4.191:55555"
)

type packer struct {
	*Packer.Packer
	privateKey *ecdsa.PrivateKey
}

type components struct {
	chain        *chain.Chain
	txpool       *txpool.TxPool
	p2p          *p2psrv.Server
	communicator *comm.Communicator
	consensus    *consensus.Consensus
	packer       *packer
	apiSrv       *http.Server
}

func main() {
	app := cli.App{
		Version:   fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit),
		Name:      "Thor",
		Usage:     "Node of VeChain Thor Network",
		Copyright: "2018 VeChain Foundation <https://vechain.org/>",
		Flags: []cli.Flag{
			p2pPortFlag,
			apiAddrFlag,
			apiCorsFlag,
			dirFlag,
			verbosityFlag,
			devFlag,
			beneficiaryFlag,
			maxPeersFlag,
		},
		Action: action,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func action(ctx *cli.Context) (err error) {
	initLogger(ctx)
	gene := selectGenesis(ctx)
	dataDir := makeDataDir(ctx, gene)

	chainDB := openChainDB(ctx, dataDir)
	defer chainDB.Close()

	logDB := openLogDB(ctx, dataDir)
	defer logDB.Close()

	components, err := makeComponent(ctx, chainDB, logDB, gene, dataDir)
	if err != nil {
		return err
	}

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())

	goes.Go(func() { runNetwork(c, components, dataDir) })
	goes.Go(func() { runAPIServer(c, components.apiSrv, ctx.String("apiaddr")) })
	goes.Go(func() { synchronizeTx(c, components) })
	goes.Go(func() { produceBlock(c, components, logDB) })

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
		log.Info("Thor network protocol used", "name", protocol.Name, "version", protocol.Version, "disc-topic", protocol.DiscTopic)
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
	log.Info("Network stoped")
}

func runAPIServer(ctx context.Context, apiSrv *http.Server, apiAddr string) {
	lsr, err := net.Listen("tcp", apiAddr)
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}
	defer lsr.Close()

	go func() {
		<-ctx.Done()
		apiSrv.Shutdown(context.TODO())
	}()

	log.Info("API service started", "listen-addr", apiAddr)
	if err := apiSrv.Serve(lsr); err != http.ErrServerClosed {
		log.Error(fmt.Sprintf("%v", err))
	}
	log.Info("API service stoped")
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
