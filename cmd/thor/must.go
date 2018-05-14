package main

import (
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/handlers"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/node"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
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
	network := ctx.String(networkFlag.Name)
	switch network {
	case "test":
		gene, err := genesis.NewTestnet()
		if err != nil {
			fatal(err)
		}
		return gene
	case "dev":
		gene, err := genesis.NewDevnet()
		if err != nil {
			fatal(err)
		}
		return gene
	default:
		cli.ShowAppHelp(ctx)
		fmt.Println("unrecognized value of -network:", network)
		os.Exit(1)
		return nil
	}
}

func makeMainDir(ctx *cli.Context) string {
	mainDir := ctx.String(dirFlag.Name)
	if mainDir == "" {
		fatal(fmt.Sprintf("unable to infer default main dir, use -%s to specify one", dirFlag.Name))
	}
	if err := os.MkdirAll(mainDir, 0700); err != nil {
		fatal(fmt.Sprintf("create main dir [%v]: %v", mainDir, err))
	}
	return mainDir
}

func makeDataDir(ctx *cli.Context, gene *genesis.Genesis) string {
	mainDir := makeMainDir(ctx)

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

func loadNodeMaster(ctx *cli.Context) *node.Master {
	mainDir := makeMainDir(ctx)
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

	if ctx.String(networkFlag.Name) == "dev" {
		i := rand.Intn(len(genesis.DevAccounts()))
		acc := genesis.DevAccounts()[i]
		return &node.Master{
			PrivateKey:  acc.PrivateKey,
			Beneficiary: bene(acc.Address),
		}
	}
	key, err := loadOrGeneratePrivateKey(filepath.Join(mainDir, "master.key"))
	if err != nil {
		fatal("load or generate master key:", err)
	}
	master := &node.Master{PrivateKey: key}
	master.Beneficiary = master.Address()
	return master
}

type p2pComm struct {
	comm      *comm.Communicator
	p2pSrv    *p2psrv.Server
	savePeers func()
}

func startP2PComm(ctx *cli.Context, chain *chain.Chain, txPool *txpool.TxPool) *p2pComm {
	mainDir := makeMainDir(ctx)
	key, err := loadOrGeneratePrivateKey(filepath.Join(mainDir, "p2p.key"))
	if err != nil {
		fatal("load or generate P2P key:", err)
	}

	nat, err := nat.Parse(ctx.String(natFlag.Name))
	if err != nil {
		cli.ShowAppHelp(ctx)
		fmt.Println("parse -nat flag:", err)
		os.Exit(1)
	}
	opts := &p2psrv.Options{
		Name:           common.MakeName("thor", fullVersion()),
		PrivateKey:     key,
		MaxPeers:       ctx.Int(maxPeersFlag.Name),
		ListenAddr:     fmt.Sprintf(":%v", ctx.Int(p2pPortFlag.Name)),
		BootstrapNodes: bootstrapNodes,
		NAT:            nat,
	}

	peersCachePath := filepath.Join(mainDir, "peers.cache")

	if data, err := ioutil.ReadFile(peersCachePath); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("failed to load peers cache", "err", err)
		}
	} else if err := rlp.DecodeBytes(data, &opts.KnownNodes); err != nil {
		log.Warn("failed to load peers cache", "err", err)
	}
	srv := p2psrv.New(opts)

	comm := comm.New(chain, txPool)
	if err := srv.Start(comm.Protocols()); err != nil {
		fatal("start P2P server:", err)
	}
	comm.Start()

	return &p2pComm{
		comm:   comm,
		p2pSrv: srv,
		savePeers: func() {
			nodes := srv.KnownNodes()
			data, err := rlp.EncodeToBytes(nodes)
			if err != nil {
				log.Warn("failed to encode cached peers", "err", err)
				return
			}
			if err := ioutil.WriteFile(peersCachePath, data, 0600); err != nil {
				log.Warn("failed to write peers cache", "err", err)
			}
		},
	}
}

func (c *p2pComm) Shutdown() {
	c.comm.Stop()
	log.Info("stopping communicator...")

	c.p2pSrv.Stop()
	log.Info("stopping P2P server...")

	c.savePeers()
	log.Info("saving peers cache...")
}

func startAPIServer(ctx *cli.Context, handler http.Handler) (*http.Server, string) {
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
	srv := &http.Server{Handler: handler}
	go func() {
		srv.Serve(listener)
	}()
	return srv, "http://" + listener.Addr().String() + "/"
}

func printStartupMessage(
	gene *genesis.Genesis,
	chain *chain.Chain,
	master *node.Master,
	dataDir string,
	apiURL string,
) {
	bestBlock := chain.BestBlock()

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

func soloGenesis(ctx *cli.Context) *genesis.Genesis {
	gene, err := genesis.NewDevnet()
	if err != nil {
		fatal(err)
	}
	return gene
}

func openMemMainDB() *lvldb.LevelDB {
	db, err := lvldb.NewMem()
	if err != nil {
		fatal(fmt.Sprintf("open chain database: %v", err))
	}
	return db
}

func openMemLogDB() *logdb.LogDB {
	db, err := logdb.NewMem()
	if err != nil {
		fatal(fmt.Sprintf("open log database: %v", err))
	}
	return db
}

func printSoloStartupMessage(
	gene *genesis.Genesis,
	chain *chain.Chain,
	dataDir string,
	apiURL string,
) {
	tableHead := `
┌────────────────────────────────────────────┬────────────────────────────────────────────────────────────────────┬──────────┬──────────┐
│                   Address                  │                             Private Key                            │    VET   │  Energy  │`
	tableContent := `
├────────────────────────────────────────────┼────────────────────────────────────────────────────────────────────┼──────────┼──────────┤
│ %v │ %v │ %v │ %v │`
	tableEnd := `
└────────────────────────────────────────────┴────────────────────────────────────────────────────────────────────┴──────────┴──────────┘`

	bestBlock := chain.BestBlock()

	info := fmt.Sprintf(`Starting %v
    Network     [ %v %v ]    
    Best block  [ %v #%v @%v ]
    Data dir    [ %v ]
    API portal  [ %v ]`,
		common.MakeName("Thor solo", fullVersion()),
		gene.ID(), gene.Name(),
		bestBlock.Header().ID(), bestBlock.Header().Number(), time.Unix(int64(bestBlock.Header().Timestamp()), 0),
		dataDir,
		apiURL)

	info += tableHead

	balance := big.NewInt(10000000)
	for _, a := range genesis.DevAccounts() {
		info += fmt.Sprintf(tableContent,
			a.Address,
			thor.BytesToBytes32(crypto.FromECDSA(a.PrivateKey)),
			balance,
			balance,
		)
	}
	info += tableEnd + "\r\n"

	fmt.Print(info)
}
