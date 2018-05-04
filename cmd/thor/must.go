package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/handlers"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/node"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	cli "gopkg.in/urfave/cli.v1"
)

func initLogger(ctx *cli.Context) {
	logLevel := ctx.Int(verbosityFlag.Name)
	log15.Root().SetHandler(log15.LvlFilterHandler(log15.Lvl(logLevel), log15.StderrHandler))
	// set go-ethereum log lvl to Warn
	ethLogHandler := ethlog.NewGlogHandler(ethlog.StreamHandler(os.Stderr, ethlog.TerminalFormat(true)))
	ethLogHandler.Verbosity(ethlog.LvlWarn)
	ethlog.Root().SetHandler(ethLogHandler)
}

func selectGenesis(ctx *cli.Context) *genesis.Genesis {
	if ctx.IsSet(devFlag.Name) {
		gene, err := genesis.NewDevnet()
		if err != nil {
			fatal(err)
		}
		return gene
	}
	gene, err := genesis.NewMainnet()
	if err != nil {
		fatal(err)
	}
	return gene
}

func makeDataDir(ctx *cli.Context, gene *genesis.Genesis) string {
	mainDir := ctx.String(dirFlag.Name)
	if mainDir == "" {
		fatal(fmt.Sprintf("unable to infer default main dir, use -%s to specify one", dirFlag.Name))
	}

	dataDir := fmt.Sprintf("%v/instance-%x", mainDir, gene.ID().Bytes()[24:])
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		fatal(fmt.Sprintf("create data dir [%v]: %v", dataDir, err))
	}
	return dataDir
}

func openMainDB(ctx *cli.Context, dataDir string) *lvldb.LevelDB {
	dir := filepath.Join(dataDir, "main.db")
	db, err := lvldb.New(dir, lvldb.Options{})
	if err != nil {
		fatal(fmt.Sprintf("open chain database [%v]: %v", dir, err))
	}
	return db
}

func openLogDB(ctx *cli.Context, dataDir string) *logdb.LogDB {
	dir := filepath.Join(dataDir, "logs.db")
	db, err := logdb.New(dir)
	if err != nil {
		fatal(fmt.Sprintf("open log database [%v]: %v", dir, err))
	}
	return db
}

func initChain(gene *genesis.Genesis, mainDB *lvldb.LevelDB, logDB *logdb.LogDB) *chain.Chain {
	genesisBlock, genesisEvents, err := gene.Build(state.NewCreator(mainDB))
	if err != nil {
		fatal("build genesis block: ", err)
	}

	chain, err := chain.New(mainDB, genesisBlock)
	if err != nil {
		fatal("initialize block chain:", err)
	}

	if err := logDB.Prepare(genesisBlock.Header()).
		ForTransaction(thor.Bytes32{}, thor.Address{}).
		Insert(genesisEvents, nil).Commit(); err != nil {
		fatal("write genesis events: ", err)
	}
	return chain
}

func loadNodeMaster(ctx *cli.Context, dataDir string) *node.Master {
	bene := func(master thor.Address) thor.Address {
		beneStr := ctx.String(beneficiaryFlag.Name)
		if beneStr == "" {
			return master
		}
		bene, err := thor.ParseAddress(beneStr)
		if err != nil {
			fatal("invalid beneficiary:", err)
		}
		return bene
	}

	if ctx.IsSet(devFlag.Name) {
		i := rand.Intn(len(genesis.DevAccounts()))
		acc := genesis.DevAccounts()[i]
		return &node.Master{
			PrivateKey:  acc.PrivateKey,
			Beneficiary: bene(acc.Address),
		}
	}
	key, err := loadOrGeneratePrivateKey(filepath.Join(dataDir, "master.key"))
	if err != nil {
		fatal("load or generate master key:", err)
	}
	master := &node.Master{PrivateKey: key}
	master.Beneficiary = master.Address()
	return master
}

func startP2PServer(ctx *cli.Context, dataDir string, protocols []*p2psrv.Protocol) (*p2psrv.Server, func()) {
	key, err := loadOrGeneratePrivateKey(filepath.Join(dataDir, "p2p.key"))
	if err != nil {
		fatal("load or generate P2P key:", err)
	}

	nat, err := nat.Parse(ctx.String(natFlag.Name))
	if err != nil {
		fatal("parse nat flag:", err)
	}
	opts := &p2psrv.Options{
		Name:           common.MakeName("thor", fullVersion()),
		PrivateKey:     key,
		MaxPeers:       ctx.Int(maxPeersFlag.Name),
		ListenAddr:     fmt.Sprintf(":%v", ctx.Int(p2pPortFlag.Name)),
		BootstrapNodes: bootstrapNodes,
		NAT:            nat,
	}

	const peersCacheFile = "peers.cache"

	if data, err := ioutil.ReadFile(filepath.Join(dataDir, peersCacheFile)); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("failed to load peers cache", "err", err)
		}
	} else if err := rlp.DecodeBytes(data, &opts.GoodNodes); err != nil {
		log.Warn("failed to load peers cache", "err", err)
	}
	srv := p2psrv.New(opts)
	if err := srv.Start(protocols); err != nil {
		fatal("start P2P server:", err)
	}
	return srv, func() {
		nodes := srv.GoodNodes()
		data, err := rlp.EncodeToBytes(nodes)
		if err != nil {
			log.Warn("failed to encode cached peers", "err", err)
			return
		}
		if err := ioutil.WriteFile(filepath.Join(dataDir, peersCacheFile), data, 0600); err != nil {
			log.Warn("failed to write peers cache", "err", err)
		}
	}
}

func startAPIServer(ctx *cli.Context, handler http.Handler) *httpServer {
	addr := ctx.String(apiAddrFlag.Name)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fatal(fmt.Sprintf("listen API addr [%v]: %v", addr, err))
	}

	if origins := ctx.String(apiCorsFlag.Name); origins != "" {
		handler = handlers.CORS(
			handlers.AllowedOrigins(strings.Split(origins, ",")),
			handlers.AllowedHeaders([]string{"content-type"}),
		)(handler)
	}
	srv := &httpServer{&http.Server{Handler: handler}, listener}
	srv.Start()
	return srv
}

func printStartupMessage(
	gene *genesis.Genesis,
	chain *chain.Chain,
	master *node.Master,
	dataDir string,
	apiURL string,
) {
	bestBlock, err := chain.GetBestBlock()
	if err != nil {
		fatal("get best block:", err)
	}

	fmt.Printf(`Starting %v
    Network     [ %v %v ]    
    Best block  [ %v #%v @%v ]
    Master      [ %v ]
    Beneficiary [ %v ]
    Data dir    [ %v ]
    API portal  [ %v ]
`,
		common.MakeName("Thor", fullVersion()),
		gene.ID(), gene.Name(),
		bestBlock.Header().ID(), bestBlock.Header().Number(), time.Unix(int64(bestBlock.Header().Timestamp()), 0),
		master.Address(), master.Beneficiary,
		dataDir,
		apiURL)
}
