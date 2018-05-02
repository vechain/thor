package main

import (
	"fmt"
	"os"

	"github.com/vechain/thor/block"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/cmd/thor/node"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	version   string
	gitCommit string
	release   = "dev"
	log       = log15.New()

	bootstrapNodes = []*discover.Node{
		discover.MustParseNode("enode://b788e1d863aaea4fecef4aba4be50e59344d64f2db002160309a415ab508977b8bffb7bac3364728f9cdeab00ebdd30e8d02648371faacd0819edc27c18b2aad@106.15.4.191:55555"),
	}
)

func fullVersion() string {
	return fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
}

func main() {
	app := cli.App{
		Version:   fullVersion(),
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
		Action: defaultAction,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultAction(ctx *cli.Context) error {
	defer func() { log.Info("exited") }()

	initLogger(ctx)
	gene := selectGenesis(ctx)
	dataDir := makeDataDir(ctx, gene)

	mainDB := openMainDB(ctx, dataDir)
	defer func() { log.Info("closing main database..."); mainDB.Close() }()

	logDB := openLogDB(ctx, dataDir)
	defer func() { log.Info("closing log database..."); logDB.Close() }()

	chain := initChain(gene, mainDB, logDB)
	master := loadNodeMaster(ctx, dataDir)

	txPool := txpool.New(chain, state.NewCreator(mainDB))
	defer func() { log.Info("stopping tx pool..."); txPool.Stop() }()

	communicator := comm.New(chain, txPool)

	p2pSrv, savePeers := startP2PServer(ctx, dataDir, communicator.Protocols())
	defer func() { log.Info("saving peers cache..."); savePeers() }()
	defer func() { log.Info("stopping P2P server..."); p2pSrv.Stop() }()

	peerCh := make(chan *p2psrv.Peer)
	peerSub := p2pSrv.SubscribePeer(peerCh)
	defer peerSub.Unsubscribe()

	syncChannelPair := node.SyncChannelPair{
		make(chan []*block.Block),
		make(chan error),
	}

	communicator.Start(peerCh, func(blocks []*block.Block) error {
		syncChannelPair.Blocks <- blocks
		return <-syncChannelPair.Ack
	})

	defer func() { log.Info("stopping communicator..."); communicator.Stop() }()

	apiSrv := startAPIServer(ctx, api.New(chain, state.NewCreator(mainDB), txPool, logDB, communicator))
	defer func() { log.Info("shutting down API server..."); apiSrv.Stop() }()

	printStartupMessage(gene, chain, dataDir, "http://"+apiSrv.listener.Addr().String()+"/")

	return node.New(
		master,
		chain,
		state.NewCreator(mainDB),
		logDB,
		txPool,
		communicator).Run(handleExitSignal(), syncChannelPair)
}
