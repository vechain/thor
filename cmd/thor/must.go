// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/elastic/gosigar"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/crypto"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/node"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
	"github.com/vechain/thor/tx"
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

func selectGenesis(ctx *cli.Context) (*genesis.Genesis, thor.ForkConfig) {
	network := ctx.String(networkFlag.Name)

	if network != "" {
		switch network {
		case "test":
			gene := genesis.NewTestnet()
			return gene, thor.GetForkConfig(gene.ID())
		case "main":
			gene := genesis.NewMainnet()
			return gene, thor.GetForkConfig(gene.ID())
		default:
			file, err := os.Open(network)
			if err != nil {
				fatal(fmt.Sprintf("open genesis file: %v", err))
			}
			defer file.Close()

			decoder := json.NewDecoder(file)
			decoder.DisallowUnknownFields()

			var forkConfig = thor.NoFork
			var gen genesis.CustomGenesis
			gen.ForkConfig = &forkConfig

			if err := decoder.Decode(&gen); err != nil {
				fatal(fmt.Sprintf("decode genesis file: %v", err))
			}

			customGen, err := genesis.NewCustomNet(&gen)
			if err != nil {
				fatal(fmt.Sprintf("build genesis: %v", err))
			}

			return customGen, forkConfig
		}
	}

	cli.ShowAppHelp(ctx)
	fmt.Println("network flag not specified")
	os.Exit(1)
	return nil, thor.ForkConfig{}
}

func makeConfigDir(ctx *cli.Context) string {
	configDir := ctx.String(configDirFlag.Name)
	if configDir == "" {
		fatal(fmt.Sprintf("unable to infer default config dir, use -%s to specify", configDirFlag.Name))
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		fatal(fmt.Sprintf("create config dir [%v]: %v", configDir, err))
	}
	return configDir
}

func makeDataDir(ctx *cli.Context) string {
	dataDir := ctx.String(dataDirFlag.Name)
	if dataDir == "" {
		fatal(fmt.Sprintf("unable to infer default data dir, use -%s to specify", dataDirFlag.Name))
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		fatal(fmt.Sprintf("create data dir [%v]: %v", dataDir, err))
	}
	return dataDir
}

func makeInstanceDir(ctx *cli.Context, gene *genesis.Genesis) string {
	dataDir := makeDataDir(ctx)

	instanceDir := filepath.Join(dataDir, fmt.Sprintf("instance-%x", gene.ID().Bytes()[24:]))
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		fatal(fmt.Sprintf("create data dir [%v]: %v", instanceDir, err))
	}
	return instanceDir
}

func openMainDB(ctx *cli.Context, dataDir string) *lvldb.LevelDB {
	cacheMB := normalizeCacheSize(ctx.Int(cacheFlag.Name))
	log.Debug("cache size(MB)", "size", cacheMB)

	// go-ethereum stuff
	// Ensure Go's GC ignores the database cache for trigger percentage
	gogc := math.Max(20, math.Min(100, 100/(float64(cacheMB)/1024)))

	log.Debug("sanitize Go's GC trigger", "percent", int(gogc))
	debug.SetGCPercent(int(gogc))

	fdCache := suggestFDCache()
	log.Debug("fd cache", "n", fdCache)

	dir := filepath.Join(dataDir, "main.db")
	db, err := lvldb.New(dir, lvldb.Options{
		CacheSize:              cacheMB / 2,
		OpenFilesCacheCapacity: fdCache,
	})
	if err != nil {
		fatal(fmt.Sprintf("open chain database [%v]: %v", dir, err))
	}
	trie.SetCache(trie.NewCache(cacheMB / 2))
	return db
}

func normalizeCacheSize(sizeMB int) int {
	if sizeMB < 128 {
		sizeMB = 128
	}

	var mem gosigar.Mem
	if err := mem.Get(); err != nil {
		log.Warn("failed to get total mem:", "err", err)
	} else {
		// limit to 1/2 os physical ram
		limitMB := int(mem.Total / 1024 / 1024 / 2)
		if sizeMB > limitMB {
			sizeMB = limitMB
			log.Warn("cache size(MB) limited", "limit", limitMB)
		}
	}
	return sizeMB
}

func suggestFDCache() int {
	limit, err := fdlimit.Current()
	if err != nil {
		fatal("failed to get fd limit:", err)
	}
	if limit <= 1024 {
		log.Warn("low fd limit, increase it if possible", "limit", limit)
	}

	n := limit / 2
	if n > 5120 {
		return 5120
	}
	return n
}

func openLogDB(ctx *cli.Context, dataDir string) *logdb.LogDB {
	dir := filepath.Join(dataDir, "logs-v2.db")
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

	if err := logDB.NewTask().ForBlock(genesisBlock.Header()).
		Write(thor.Bytes32{}, thor.Address{}, []*tx.Output{{
			Events: genesisEvents,
		}}).Commit(); err != nil {
		fatal("write genesis events: ", err)
	}
	return chain
}

func masterKeyPath(ctx *cli.Context) string {
	configDir := makeConfigDir(ctx)
	return filepath.Join(configDir, "master.key")
}

func beneficiary(ctx *cli.Context) *thor.Address {
	value := ctx.String(beneficiaryFlag.Name)
	if value == "" {
		return nil
	}
	addr, err := thor.ParseAddress(value)
	if err != nil {
		fatal("invalid beneficiary:", err)
	}
	return &addr
}

func loadNodeMaster(ctx *cli.Context) *node.Master {
	if ctx.String(networkFlag.Name) == "dev" {
		i := rand.Intn(len(genesis.DevAccounts()))
		acc := genesis.DevAccounts()[i]
		return &node.Master{
			PrivateKey:  acc.PrivateKey,
			Beneficiary: beneficiary(ctx),
		}
	}
	key, err := loadOrGeneratePrivateKey(masterKeyPath(ctx))
	if err != nil {
		fatal("load or generate master key:", err)
	}
	master := &node.Master{PrivateKey: key}
	master.Beneficiary = beneficiary(ctx)
	return master
}

type p2pComm struct {
	comm           *comm.Communicator
	p2pSrv         *p2psrv.Server
	peersCachePath string
}

func newP2PComm(ctx *cli.Context, chain *chain.Chain, txPool *txpool.TxPool, instanceDir string) *p2pComm {
	configDir := makeConfigDir(ctx)
	key, err := loadOrGeneratePrivateKey(filepath.Join(configDir, "p2p.key"))
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

	bootnodes := parseBootNode(ctx)
	if bootnodes != nil {
		opts.BootstrapNodes = bootnodes
	}

	peersCachePath := filepath.Join(instanceDir, "peers.cache")

	if data, err := ioutil.ReadFile(peersCachePath); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("failed to load peers cache", "err", err)
		}
	} else if err := rlp.DecodeBytes(data, &opts.KnownNodes); err != nil {
		log.Warn("failed to load peers cache", "err", err)
	}

	var empty struct{}
	m := make(map[discover.NodeID]struct{})
	for _, node := range opts.KnownNodes {
		m[node.ID] = empty
	}
	for _, bootnode := range bootnodes {
		if _, ok := m[bootnode.ID]; !ok {
			opts.KnownNodes = append(opts.KnownNodes, bootnode)
		}
	}

	return &p2pComm{
		comm:           comm.New(chain, txPool),
		p2pSrv:         p2psrv.New(opts),
		peersCachePath: peersCachePath,
	}
}

func (p *p2pComm) Start() {
	log.Info("starting P2P networking")
	if err := p.p2pSrv.Start(p.comm.Protocols()); err != nil {
		fatal("start P2P server:", err)
	}
	p.comm.Start()
}

func (p *p2pComm) Stop() {
	log.Info("stopping communicator...")
	p.comm.Stop()

	log.Info("stopping P2P server...")
	p.p2pSrv.Stop()

	log.Info("saving peers cache...")
	nodes := p.p2pSrv.KnownNodes()
	data, err := rlp.EncodeToBytes(nodes)
	if err != nil {
		log.Warn("failed to encode cached peers", "err", err)
		return
	}
	if err := ioutil.WriteFile(p.peersCachePath, data, 0600); err != nil {
		log.Warn("failed to write peers cache", "err", err)
	}
}

func startAPIServer(ctx *cli.Context, handler http.Handler, genesisID thor.Bytes32) (string, func()) {
	addr := ctx.String(apiAddrFlag.Name)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fatal(fmt.Sprintf("listen API addr [%v]: %v", addr, err))
	}
	timeout := ctx.Int(apiTimeoutFlag.Name)
	if timeout > 0 {
		handler = handleAPITimeout(handler, time.Duration(timeout)*time.Millisecond)
	}
	handler = handleXGenesisID(handler, genesisID)
	handler = handleXThorestVersion(handler)
	handler = requestBodyLimit(handler)
	srv := &http.Server{Handler: handler}
	var goes co.Goes
	goes.Go(func() {
		srv.Serve(listener)
	})
	return "http://" + listener.Addr().String() + "/", func() {
		srv.Close()
		goes.Wait()
	}
}

func printStartupMessage1(
	gene *genesis.Genesis,
	chain *chain.Chain,
	master *node.Master,
	dataDir string,
	forkConfig thor.ForkConfig,
) {
	bestBlock := chain.BestBlock()

	fmt.Printf(`Starting %v
    Network      [ %v %v ]
    Best block   [ %v #%v @%v ]
    Forks        [ %v ]
    Master       [ %v ]
    Beneficiary  [ %v ]
    Instance dir [ %v ]
`,
		common.MakeName("Thor", fullVersion()),
		gene.ID(), gene.Name(),
		bestBlock.Header().ID(), bestBlock.Header().Number(), time.Unix(int64(bestBlock.Header().Timestamp()), 0),
		forkConfig,
		master.Address(),
		func() string {
			if master.Beneficiary == nil {
				return "not set, defaults to endorsor"
			}
			return master.Beneficiary.String()
		}(),
		dataDir)
}

func printStartupMessage2(
	apiURL string,
	nodeID string,
) {
	fmt.Printf(`    API portal   [ %v ]
    Node ID      [ %v ]
`,
		apiURL,
		nodeID)
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
	forkConfig thor.ForkConfig,
) {
	tableHead := `
┌────────────────────────────────────────────┬────────────────────────────────────────────────────────────────────┐
│                   Address                  │                             Private Key                            │`
	tableContent := `
├────────────────────────────────────────────┼────────────────────────────────────────────────────────────────────┤
│ %v │ %v │`
	tableEnd := `
└────────────────────────────────────────────┴────────────────────────────────────────────────────────────────────┘`

	bestBlock := chain.BestBlock()

	info := fmt.Sprintf(`Starting %v
    Network     [ %v %v ]    
    Best block  [ %v #%v @%v ]
    Forks       [ %v ]
    Data dir    [ %v ]
    API portal  [ %v ]`,
		common.MakeName("Thor solo", fullVersion()),
		gene.ID(), gene.Name(),
		bestBlock.Header().ID(), bestBlock.Header().Number(), time.Unix(int64(bestBlock.Header().Timestamp()), 0),
		forkConfig,
		dataDir,
		apiURL)

	info += tableHead

	for _, a := range genesis.DevAccounts() {
		info += fmt.Sprintf(tableContent,
			a.Address,
			thor.BytesToBytes32(crypto.FromECDSA(a.PrivateKey)),
		)
	}
	info += tableEnd + "\r\n"

	fmt.Print(info)
}

func getNodeID(ctx *cli.Context) string {
	configDir := makeConfigDir(ctx)
	key, err := loadOrGeneratePrivateKey(filepath.Join(configDir, "p2p.key"))
	if err != nil {
		fatal("load or generate P2P key:", err)
	}

	return fmt.Sprintf("enode://%x@[extip]:%v", discover.PubkeyID(&key.PublicKey).Bytes(), ctx.Int(p2pPortFlag.Name))
}

func parseBootNode(ctx *cli.Context) []*discover.Node {
	s := strings.TrimSpace(ctx.String(bootNodeFlag.Name))
	if s == "" {
		return nil
	}
	inputs := strings.Split(s, ",")
	var nodes []*discover.Node
	for _, i := range inputs {
		node := discover.MustParseNode(i)
		nodes = append(nodes, node)
	}
	return nodes
}
