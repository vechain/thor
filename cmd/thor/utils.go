// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/elastic/gosigar"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/mattn/go-tty"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/doc"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/node"
	"github.com/vechain/thor/v2/cmd/thor/p2p"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/p2psrv"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
	"gopkg.in/urfave/cli.v1"

	ethlog "github.com/ethereum/go-ethereum/log"
)

var devNetGenesisID = genesis.NewDevnet().ID()

func initLogger(ctx *cli.Context) {
	logLevel := ctx.Int(verbosityFlag.Name)
	log15.Root().SetHandler(log15.LvlFilterHandler(log15.Lvl(logLevel), log15.StderrHandler))
	// set go-ethereum log lvl to Warn
	ethLogHandler := ethlog.NewGlogHandler(ethlog.StreamHandler(os.Stderr, ethlog.TerminalFormat(true)))
	ethLogHandler.Verbosity(ethlog.LvlWarn)
	ethlog.Root().SetHandler(ethLogHandler)
}

func loadOrGeneratePrivateKey(path string) (*ecdsa.PrivateKey, error) {
	key, err := crypto.LoadECDSA(path)
	if err == nil {
		return key, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	key, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	if err := crypto.SaveECDSA(path, key); err != nil {
		return nil, err
	}
	return key, nil
}

func defaultConfigDir() string {
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".org.vechain.thor")
	}
	return ""
}

// copy from go-ethereum
func defaultDataDir() string {
	// Try to place the data folder in the user's home dir
	if home := homeDir(); home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support", "org.vechain.thor")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "org.vechain.thor")
		} else {
			return filepath.Join(home, ".org.vechain.thor")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

func handleExitSignal() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		exitSignalCh := make(chan os.Signal, 1)
		signal.Notify(exitSignalCh, os.Interrupt, syscall.SIGTERM)

		sig := <-exitSignalCh
		log.Info("exit signal received", "signal", sig)
		cancel()
	}()
	return ctx
}

// middleware to limit request body size.
func requestBodyLimit(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 200*1024)
		h.ServeHTTP(w, r)
	})
}

// middleware to verify 'x-genesis-id' header in request, and set to response headers.
func handleXGenesisID(h http.Handler, genesisID thor.Bytes32) http.Handler {
	const headerKey = "x-genesis-id"
	expectedID := genesisID.String()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actualID := r.Header.Get(headerKey)
		if actualID == "" {
			actualID = r.URL.Query().Get(headerKey)
		}
		w.Header().Set(headerKey, expectedID)
		if actualID != "" && actualID != expectedID {
			io.Copy(io.Discard, r.Body)
			http.Error(w, "genesis id mismatch", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// middleware to set 'x-thorest-ver' to response headers.
func handleXThorestVersion(h http.Handler) http.Handler {
	const headerKey = "x-thorest-ver"
	ver := doc.Version()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerKey, ver)
		h.ServeHTTP(w, r)
	})
}

// middleware for http request timeout.
func handleAPITimeout(h http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func readPasswordFromNewTTY(prompt string) (string, error) {
	t, err := tty.Open()
	if err != nil {
		return "", err
	}
	defer t.Close()
	fmt.Fprint(t.Output(), prompt)
	pass, err := t.ReadPasswordNoEcho()
	if err != nil {
		return "", err
	}
	return pass, err
}

func selectGenesis(ctx *cli.Context) (*genesis.Genesis, thor.ForkConfig, error) {
	network := ctx.String(networkFlag.Name)
	if network == "" {
		_ = cli.ShowAppHelp(ctx)
		return nil, thor.ForkConfig{}, errors.New("network flag not specified")
	}

	switch network {
	case "test":
		gene := genesis.NewTestnet()
		return gene, thor.GetForkConfig(gene.ID()), nil
	case "main":
		gene := genesis.NewMainnet()
		return gene, thor.GetForkConfig(gene.ID()), nil
	default:
		return parseGenesisFile(network)
	}
}

func parseGenesisFile(filePath string) (*genesis.Genesis, thor.ForkConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, thor.ForkConfig{}, errors.Wrap(err, "open genesis file")
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var forkConfig = thor.NoFork
	var gen genesis.CustomGenesis
	gen.ForkConfig = &forkConfig

	if err := decoder.Decode(&gen); err != nil {
		return nil, thor.ForkConfig{}, errors.Wrap(err, "decode genesis file")
	}

	customGen, err := genesis.NewCustomNet(&gen)
	if err != nil {
		return nil, thor.ForkConfig{}, errors.Wrap(err, "build genesis")
	}

	return customGen, forkConfig, nil
}

func makeConfigDir(ctx *cli.Context) (string, error) {
	dir := ctx.String(configDirFlag.Name)
	if dir == "" {
		return "", fmt.Errorf("unable to infer default config dir, use -%s to specify", configDirFlag.Name)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", errors.Wrapf(err, "create config dir [%v]", dir)
	}
	return dir, nil
}

func makeInstanceDir(ctx *cli.Context, gene *genesis.Genesis) (string, error) {
	dataDir := ctx.String(dataDirFlag.Name)
	if dataDir == "" {
		return "", fmt.Errorf("unable to infer default data dir, use -%s to specify", dataDirFlag.Name)
	}

	suffix := ""
	if ctx.Bool(disablePrunerFlag.Name) {
		suffix = "-full"
	}

	instanceDir := filepath.Join(dataDir, fmt.Sprintf("instance-%x-v3", gene.ID().Bytes()[24:])+suffix)
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		return "", errors.Wrapf(err, "create instance dir [%v]", instanceDir)
	}
	return instanceDir, nil
}

func openMainDB(ctx *cli.Context, dir string) (*muxdb.MuxDB, error) {
	cacheMB := normalizeCacheSize(ctx.Int(cacheFlag.Name))
	log.Debug("cache size(MB)", "size", cacheMB)

	fdCache := suggestFDCache()
	log.Debug("fd cache", "n", fdCache)

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        cacheMB,
		TrieRootCacheCapacity:      256,
		TrieCachedNodeTTL:          30, // 5min
		TrieLeafBankSlotCapacity:   256,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       !ctx.Bool(disablePrunerFlag.Name),
		OpenFilesCacheCapacity:     fdCache,
		ReadCacheMB:                256, // rely on os page cache other than huge db read cache.
		WriteBufferMB:              128,
	}

	// go-ethereum stuff
	// Ensure Go's GC ignores the database cache for trigger percentage
	totalCacheMB := cacheMB + opts.ReadCacheMB + opts.WriteBufferMB*2
	gogc := math.Max(10, math.Min(100, 50/(float64(totalCacheMB)/1024)))

	log.Debug("sanitize Go's GC trigger", "percent", int(gogc))
	debug.SetGCPercent(int(gogc))

	if opts.TrieWillCleanHistory {
		opts.TrieHistPartitionFactor = 1000
	} else {
		opts.TrieHistPartitionFactor = 500000
	}

	path := filepath.Join(dir, "main.db")
	db, err := muxdb.Open(path, &opts)
	if err != nil {
		return nil, errors.Wrapf(err, "open main database [%v]", path)
	}
	return db, nil
}

func normalizeCacheSize(sizeMB int) int {
	if sizeMB < 128 {
		sizeMB = 128
	}

	var mem gosigar.Mem
	if err := mem.Get(); err != nil {
		log.Warn("failed to get total mem:", "err", err)
	} else {
		total := int(mem.Total / 1024 / 1024)
		half := total / 2

		// limit to not less than total/2 and up to total-2GB
		limitMB := total - 2048
		if limitMB < half {
			limitMB = half
		}

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
		log.Warn("unable to get fdlimit", "error", err)
		return 500
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

func openLogDB(dir string) (*logdb.LogDB, error) {
	path := filepath.Join(dir, "logs.db")
	db, err := logdb.New(path)
	if err != nil {
		return nil, errors.Wrapf(err, "open log database [%v]", path)
	}
	return db, nil
}

func initChainRepository(gene *genesis.Genesis, mainDB *muxdb.MuxDB, logDB *logdb.LogDB) (*chain.Repository, error) {
	genesisBlock, genesisEvents, genesisTransfers, err := gene.Build(state.NewStater(mainDB))
	if err != nil {
		return nil, errors.Wrap(err, "build genesis block")
	}

	repo, err := chain.NewRepository(mainDB, genesisBlock)
	if err != nil {
		return nil, errors.Wrap(err, "initialize block chain")
	}
	w := logDB.NewWriter()
	if err := w.Write(genesisBlock, tx.Receipts{{
		Outputs: []*tx.Output{
			{Events: genesisEvents, Transfers: genesisTransfers},
		},
	}}); err != nil {
		return nil, errors.Wrap(err, "write genesis logs")
	}
	if err := w.Commit(); err != nil {
		return nil, errors.Wrap(err, "commit genesis logs")
	}
	return repo, nil
}

func beneficiary(ctx *cli.Context) (*thor.Address, error) {
	value := ctx.String(beneficiaryFlag.Name)
	if value == "" {
		return nil, nil
	}
	addr, err := thor.ParseAddress(value)
	if err != nil {
		return nil, errors.Wrap(err, "invalid beneficiary")
	}
	return &addr, nil
}

func masterKeyPath(ctx *cli.Context) (string, error) {
	configDir, err := makeConfigDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "master.key"), nil
}

func loadNodeMaster(ctx *cli.Context) (*node.Master, error) {
	path, err := masterKeyPath(ctx)
	if err != nil {
		return nil, err
	}
	key, err := loadOrGeneratePrivateKey(path)
	if err != nil {
		return nil, errors.Wrap(err, "load or generate master key")
	}
	master := &node.Master{PrivateKey: key}
	if master.Beneficiary, err = beneficiary(ctx); err != nil {
		return nil, err
	}
	return master, nil
}

func newP2PCommunicator(ctx *cli.Context, repo *chain.Repository, txPool *txpool.TxPool, instanceDir string) (*p2p.P2P, error) {
	// known peers will be loaded/stored from/in this file
	peersCachePath := filepath.Join(instanceDir, "peers.cache")

	configDir, err := makeConfigDir(ctx)
	if err != nil {
		return nil, err
	}

	key, err := loadOrGeneratePrivateKey(filepath.Join(configDir, "p2p.key"))
	if err != nil {
		return nil, errors.Wrap(err, "load or generate P2P key")
	}

	userNAT, err := nat.Parse(ctx.String(natFlag.Name))
	if err != nil {
		cli.ShowAppHelp(ctx)
		return nil, errors.Wrap(err, "parse -nat flag")
	}

	allowedPeers, err := parseNodeList(strings.TrimSpace(ctx.String(allowedPeersFlag.Name)))
	if err != nil {
		return nil, fmt.Errorf("unable to parse allowed peers - %w", err)
	}

	bootnodePeers, err := parseNodeList(strings.TrimSpace(ctx.String(bootNodeFlag.Name)))
	if err != nil {
		return nil, fmt.Errorf("unable to parse bootnode peers - %w", err)
	}

	var cachedPeers p2psrv.Nodes
	if data, err := os.ReadFile(peersCachePath); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("failed to load peers cache", "err", err)
		}
	} else if err := rlp.DecodeBytes(data, &cachedPeers); err != nil {
		log.Warn("failed to load peers cache", "err", err)
	}

	return p2p.New(
		comm.New(repo, txPool),
		key,
		instanceDir,
		userNAT,
		fullVersion(),
		ctx.Int(maxPeersFlag.Name),
		ctx.Int(p2pPortFlag.Name),
		fmt.Sprintf(":%v", ctx.Int(p2pPortFlag.Name)),
		allowedPeers,
		cachedPeers,
		bootnodePeers,
	), nil
}

func startAPIServer(ctx *cli.Context, handler http.Handler, genesisID thor.Bytes32) (string, func(), error) {
	addr := ctx.String(apiAddrFlag.Name)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen API addr [%v]", addr)
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
	}, nil
}

func printStartupMessage1(
	gene *genesis.Genesis,
	repo *chain.Repository,
	master *node.Master,
	dataDir string,
	forkConfig thor.ForkConfig,
) {
	bestBlock := repo.BestBlockSummary()

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
		bestBlock.Header.ID(), bestBlock.Header.Number(), time.Unix(int64(bestBlock.Header.Timestamp()), 0),
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

func openMemMainDB() *muxdb.MuxDB {
	return muxdb.NewMem()
}

func openMemLogDB() *logdb.LogDB {
	db, err := logdb.NewMem()
	if err != nil {
		panic(errors.Wrap(err, "open log database"))
	}
	return db
}

func printSoloStartupMessage(
	gene *genesis.Genesis,
	repo *chain.Repository,
	dataDir string,
	apiURL string,
	forkConfig thor.ForkConfig,
) {
	bestBlock := repo.BestBlockSummary()

	info := fmt.Sprintf(`Starting %v
    Network     [ %v %v ]    
    Best block  [ %v #%v @%v ]
    Forks       [ %v ]
    Data dir    [ %v ]
    API portal  [ %v ]
`,
		common.MakeName("Thor solo", fullVersion()),
		gene.ID(), gene.Name(),
		bestBlock.Header.ID(), bestBlock.Header.Number(), time.Unix(int64(bestBlock.Header.Timestamp()), 0),
		forkConfig,
		dataDir,
		apiURL)

	if gene.ID() == devNetGenesisID {
		info += `┌──────────────────┬───────────────────────────────────────────────────────────────────────────────┐
│  Mnemonic Words  │  denial kitchen pet squirrel other broom bar gas better priority spoil cross  │
└──────────────────┴───────────────────────────────────────────────────────────────────────────────┘
`
	}

	fmt.Print(info)
}

func parseNodeList(list string) ([]*discover.Node, error) {
	inputs := strings.Split(list, ",")
	var nodes []*discover.Node
	for _, i := range inputs {
		if i == "" {
			continue
		}
		node, err := discover.ParseNode(i)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}
