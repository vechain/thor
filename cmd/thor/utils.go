// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/elastic/gosigar"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/crypto"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-tty"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/httpserver"
	"github.com/vechain/thor/v2/cmd/thor/node"
	"github.com/vechain/thor/v2/cmd/thor/p2p"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/p2psrv"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
	"gopkg.in/urfave/cli.v1"
)

var devNetGenesisID thor.Bytes32

func initLogger(lvl int, jsonLogs bool) *slog.LevelVar {
	logLevel := log.FromLegacyLevel(lvl)
	output := io.Writer(os.Stdout)
	var level slog.LevelVar
	level.Set(logLevel)

	var handler slog.Handler
	if jsonLogs {
		handler = log.JSONHandlerWithLevel(output, &level)
	} else {
		useColor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
		handler = log.NewTerminalHandlerWithLevel(output, &level, useColor)
	}
	log.SetDefault(log.NewLogger(handler))
	ethlog.Root().SetHandler(ethlog.LvlFilterHandler(ethlog.LvlWarn, &ethLogger{
		logger: log.WithContext("pkg", "geth"),
	}))

	return &level
}

type ethLogger struct {
	logger log.Logger
}

func (h *ethLogger) Log(r *ethlog.Record) error {
	switch r.Lvl {
	case ethlog.LvlCrit:
		h.logger.Crit(r.Msg)
	case ethlog.LvlError:
		h.logger.Error(r.Msg)
	case ethlog.LvlWarn:
		h.logger.Warn(r.Msg)
	case ethlog.LvlInfo:
		h.logger.Info(r.Msg)
	case ethlog.LvlDebug:
		h.logger.Debug(r.Msg)
	case ethlog.LvlTrace:
		h.logger.Trace(r.Msg)
	default:
		return nil
	}
	return nil
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

func selectGenesis(ctx *cli.Context) (*genesis.Genesis, *thor.ForkConfig, error) {
	network := ctx.String(networkFlag.Name)
	if network == "" {
		_ = cli.ShowAppHelp(ctx)
		return nil, nil, errors.New("network flag not specified")
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

func parseGenesisFile(uri string) (*genesis.Genesis, *thor.ForkConfig, error) {
	var (
		reader io.ReadCloser
		err    error
	)
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		res, err := http.Get(uri) // #nosec
		if err != nil {
			return nil, nil, errors.Wrap(err, "http get genesis file")
		}
		reader = res.Body
	} else {
		reader, err = os.Open(uri)
		if err != nil {
			return nil, nil, errors.Wrap(err, "open genesis file")
		}
	}
	defer reader.Close()

	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var forkConfig = thor.NoFork
	var gen genesis.CustomGenesis
	gen.ForkConfig = &forkConfig

	if err := decoder.Decode(&gen); err != nil {
		return nil, nil, errors.Wrap(err, "decode genesis file")
	}

	customGen, err := genesis.NewCustomNet(&gen)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build genesis")
	}

	return customGen, &forkConfig, nil
}

func makeAPIConfig(ctx *cli.Context, logAPIRequests *atomic.Bool, soloMode bool) httpserver.APIConfig {
	return httpserver.APIConfig{
		AllowedOrigins:             ctx.String(apiCorsFlag.Name),
		BacktraceLimit:             uint32(ctx.Uint64(apiBacktraceLimitFlag.Name)),
		CallGasLimit:               ctx.Uint64(apiCallGasLimitFlag.Name),
		PprofOn:                    ctx.Bool(pprofFlag.Name),
		SkipLogs:                   ctx.Bool(skipLogsFlag.Name),
		APIBacktraceLimit:          int(ctx.Uint64(apiBacktraceLimitFlag.Name)),
		PriorityIncreasePercentage: int(ctx.Uint64(apiPriorityFeesPercentageFlag.Name)),
		AllowCustomTracer:          ctx.Bool(apiAllowCustomTracerFlag.Name),
		EnableReqLogger:            logAPIRequests,
		EnableMetrics:              ctx.Bool(enableMetricsFlag.Name),
		LogsLimit:                  ctx.Uint64(apiLogsLimitFlag.Name),
		AllowedTracers:             parseTracerList(strings.TrimSpace(ctx.String(allowedTracersFlag.Name))),
		EnableDeprecated:           ctx.Bool(apiEnableDeprecatedFlag.Name),
		SoloMode:                   soloMode,
		EnableTxPool:               ctx.Bool(apiTxpoolFlag.Name),
		Timeout:                    ctx.Int(apiTimeoutFlag.Name),
	}
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

	instanceDir := filepath.Join(dataDir, fmt.Sprintf("instance-%x-v4", gene.ID().Bytes()[24:])+suffix)
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
		TrieCachedNodeTTL:          30, // 5min
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
		opts.TrieHistPartitionFactor = 256
	} else {
		opts.TrieHistPartitionFactor = 524288
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
		limitMB := max(total-2048, half)

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
	path := filepath.Join(dir, "logs-v2.db")
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

func loadNodeMasterFromStdin() (*ecdsa.PrivateKey, error) {
	var (
		input string
		err   error
	)
	if isatty.IsTerminal(os.Stdin.Fd()) {
		input, err = readPasswordFromNewTTY("Enter master key: ")
		if err != nil {
			return nil, err
		}
	} else {
		reader := bufio.NewReader(os.Stdin)
		input, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
	}

	return crypto.HexToECDSA(strings.TrimSpace(input))
}

func loadNodeMaster(ctx *cli.Context) (*node.Master, error) {
	var key *ecdsa.PrivateKey
	var err error

	useStdin := ctx.Bool(masterKeyStdinFlag.Name)
	if useStdin {
		key, err = loadNodeMasterFromStdin()
		if err != nil {
			return nil, errors.Wrap(err, "read master key from stdin")
		}
	} else {
		path, err := masterKeyPath(ctx)
		if err != nil {
			return nil, err
		}
		key, err = loadOrGeneratePrivateKey(path)
		if err != nil {
			return nil, errors.Wrap(err, "load or generate master key")
		}
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

func printStartupMessage1(
	gene *genesis.Genesis,
	repo *chain.Repository,
	master *node.Master,
	dataDir string,
	forkConfig *thor.ForkConfig,
) {
	bestBlock := repo.BestBlockSummary()

	name := common.MakeName("Thor", fullVersion())
	if master == nil { // solo has no master
		name = common.MakeName("Thor solo", fullVersion())
	}

	fmt.Printf(`Starting %v
    Network      [ %v %v ]
    Best block   [ %v #%v @%v ]
    Forks        [ %v ]%v
    Instance dir [ %v ]
`,
		name,
		gene.ID(), gene.Name(),
		bestBlock.Header.ID(), bestBlock.Header.Number(), time.Unix(int64(bestBlock.Header.Timestamp()), 0),
		forkConfig,
		func() string {
			// solo mode does not have master, so skip this part
			if master == nil {
				return ""
			} else {
				return fmt.Sprintf(`
    Master       [ %v ]
    Beneficiary  [ %v ]`,
					master.Address(),
					func() string {
						if master.Beneficiary == nil {
							return "not set, defaults to endorsor"
						}
						return master.Beneficiary.String()
					}(),
				)
			}
		}(),
		dataDir,
	)
}

func getOrCreateDevnetID() thor.Bytes32 {
	if devNetGenesisID.IsZero() {
		devNetGenesisID = genesis.NewDevnet().ID()
	}
	return devNetGenesisID
}

func printStartupMessage2(
	gene *genesis.Genesis,
	apiURL string,
	nodeID string,
	metricsURL string,
	adminURL string,
) {
	fmt.Printf(`%v    API portal   [ %v ]%v%v%v`,
		func() string { // node ID
			if nodeID == "" {
				return ""
			} else {
				return fmt.Sprintf(`    Node ID      [ %v ]
`,
					nodeID)
			}
		}(),
		apiURL,
		func() string { // metrics URL
			if metricsURL == "" {
				return ""
			} else {
				return fmt.Sprintf(`
    Metrics      [ %v ]`,
					metricsURL)
			}
		}(),
		func() string { // admin URL
			if adminURL == "" {
				return ""
			} else {
				return fmt.Sprintf(`
    Admin        [ %v ]`,
					adminURL)
			}
		}(),
		func() string {
			// print default dev net's dev accounts info
			if gene.ID() == getOrCreateDevnetID() {
				return `
┌──────────────────┬───────────────────────────────────────────────────────────────────────────────┐
│  Mnemonic Words  │  denial kitchen pet squirrel other broom bar gas better priority spoil cross  │
└──────────────────┴───────────────────────────────────────────────────────────────────────────────┘
`
			} else {
				return "\n"
			}
		}(),
	)
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

func readIntFromUInt64Flag(val uint64) (int, error) {
	i := int(val)

	if i < 0 {
		return 0, fmt.Errorf("invalid value %d ", val)
	}

	return i, nil
}

func parseTracerList(list string) []string {
	inputs := strings.Split(list, ",")
	tracers := make([]string, 0, len(inputs))

	for _, i := range inputs {
		name := strings.TrimSpace(i)
		if name == "" {
			continue
		}
		if name == "none" {
			return []string{}
		}
		if name == "all" {
			return []string{"all"}
		}

		tracers = append(tracers, i)
	}

	return tracers
}
