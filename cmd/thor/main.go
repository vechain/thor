package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/cmd/thor/node"
	"github.com/vechain/thor/cmd/thor/solo"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
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
		discover.MustParseNode("enode://2edf9df89736a646cf4b95921f2cc0cd62779f59e5202163c63134e194215d68fd58050a9fe434942943f07bcc2f088a6eb270d58e7fc29da6b01bd56e103759@139.224.162.220:55555"),
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
			dirFlag,
			networkFlag,
			beneficiaryFlag,
			apiAddrFlag,
			apiCorsFlag,
			verbosityFlag,
			maxPeersFlag,
			p2pPortFlag,
			natFlag,
		},
		Action: defaultAction,
		Commands: []cli.Command{
			{
				Name:  "solo",
				Usage: "VeChain Thor client for test & dev",
				Flags: []cli.Flag{
					dirFlag,
					apiAddrFlag,
					apiCorsFlag,
					onDemandFlag,
					persistFlag,
					verbosityFlag,
				},
				Action: soloAction,
			},
		},
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
	master := loadNodeMaster(ctx)

	txPool := txpool.New(chain, state.NewCreator(mainDB))
	defer func() { log.Info("closing tx pool..."); txPool.Close() }()

	p2pcom := startP2PComm(ctx, chain, txPool)
	defer p2pcom.Shutdown()

	apiSrv, apiURL := startAPIServer(ctx, api.New(chain, state.NewCreator(mainDB), txPool, logDB, p2pcom.comm))
	defer func() { log.Info("stopping API server..."); apiSrv.Shutdown(context.Background()) }()

	printStartupMessage(gene, chain, master, dataDir, apiURL)

	return node.New(master, chain, state.NewCreator(mainDB), logDB, txPool, p2pcom.comm).
		Run(handleExitSignal())
}

func soloAction(ctx *cli.Context) error {
	defer func() { log.Info("exited") }()

	initLogger(ctx)
	gene := soloGenesis(ctx)

	var mainDB *lvldb.LevelDB
	var logDB *logdb.LogDB
	var dataDir string

	if ctx.Bool("persist") {
		dataDir = makeDataDir(ctx, gene)
		mainDB = openMainDB(ctx, dataDir)
		logDB = openLogDB(ctx, dataDir)
	} else {
		dataDir = "Memory"
		mainDB = openMemMainDB()
		logDB = openMemLogDB()
	}

	defer func() { log.Info("closing main database..."); mainDB.Close() }()
	defer func() { log.Info("closing log database..."); logDB.Close() }()

	chain := initChain(gene, mainDB, logDB)

	txPool := txpool.New(chain, state.NewCreator(mainDB))
	defer func() { log.Info("closing tx pool..."); txPool.Close() }()

	soloContext := solo.New(chain, state.NewCreator(mainDB), logDB, txPool, ctx.Bool("on-demand"))

	apiSrv, apiURL := startAPIServer(ctx, api.New(chain, state.NewCreator(mainDB), txPool, logDB, solo.Communicator{}))
	defer func() { log.Info("stopping API server..."); apiSrv.Shutdown(context.Background()) }()

	printSoloStartupMessage(gene, chain, dataDir, apiURL)

	return soloContext.Run(handleExitSignal())
}
