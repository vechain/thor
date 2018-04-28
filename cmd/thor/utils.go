package main

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/ethereum/go-ethereum/crypto"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/eventdb"
	"github.com/vechain/thor/genesis"
	Genesis "github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	Lvldb "github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	Packer "github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

func fatal(args ...interface{}) {
	var w io.Writer
	if runtime.GOOS == "windows" {
		// The SameFile check below doesn't work on Windows.
		// stdout is unlikely to get redirected though, so just print there.
		w = os.Stdout
	} else {
		outf, _ := os.Stdout.Stat()
		errf, _ := os.Stderr.Stat()
		if outf != nil && errf != nil && os.SameFile(outf, errf) {
			w = os.Stderr
		} else {
			w = io.MultiWriter(os.Stdout, os.Stderr)
		}
	}
	fmt.Print(w, "Fatal:")
	fmt.Fprintln(w, args...)
	os.Exit(1)
}

func fatalf(format string, args ...interface{}) {
	fatal(fmt.Sprintf(format, args...))
}

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

func loadKey(keyFile string) (key *ecdsa.PrivateKey, err error) {
	// try to load from file
	if key, err = crypto.LoadECDSA(keyFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		return key, nil
	}

	// no such file, generate new key and write in
	key, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	if err := crypto.SaveECDSA(keyFile, key); err != nil {
		return nil, err
	}
	return key, nil
}

func loadProposer(isDev bool, keyFile string) (thor.Address, *ecdsa.PrivateKey, error) {
	if isDev {
		index := rand.Intn(len(Genesis.DevAccounts()))
		return Genesis.DevAccounts()[index].Address, Genesis.DevAccounts()[index].PrivateKey, nil
	}

	key, err := loadKey(keyFile)
	if err != nil {
		return thor.Address{}, nil, err
	}
	return thor.Address(crypto.PubkeyToAddress(key.PublicKey)), key, nil
}

func makeDataDir(ctx *cli.Context, gene *Genesis.Genesis) string {
	mainDir := ctx.String(dirFlag.Name)
	if mainDir == "" {
		fatalf("unable to infer default main dir, use -%s to specify one", dirFlag.Name)
	}

	dataDir := fmt.Sprintf("%v/instance-%x", mainDir, gene.ID().Bytes()[24:])
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		fatalf("create data dir at '%v': %v", dataDir, err)
	}
	return dataDir
}

func openChainDB(ctx *cli.Context, dataDir string) *lvldb.LevelDB {
	dir := filepath.Join(dataDir, "chain.db")
	db, err := Lvldb.New(dir, Lvldb.Options{})
	if err != nil {
		fatalf("open chain database at '%v': %v", dir, err)
	}
	return db
}

func openEventDB(ctx *cli.Context, dataDir string) *eventdb.EventDB {
	dir := filepath.Join(dataDir, "event.db")
	db, err := eventdb.New(dir)
	if err != nil {
		fatal("open event database at '%v': %v", dir, err)
	}
	return db
}

func openTransferDB(ctx *cli.Context, dataDir string) *transferdb.TransferDB {
	dir := filepath.Join(dataDir, "transfer.db")
	db, err := transferdb.New(dir)
	if err != nil {
		fatal("open transfer database at '%v': %v", dir, err)
	}
	return db
}

func makeComponent(
	ctx *cli.Context,
	lvldb *Lvldb.LevelDB,
	eventDB *eventdb.EventDB,
	transferDB *transferdb.TransferDB,
	genesis *Genesis.Genesis,
	dataDir string,
) (*components, error) {
	stateCreator := state.NewCreator(lvldb)

	genesisBlock, blockEvents, err := genesis.Build(stateCreator)
	if err != nil {
		return nil, err
	}

	var events []*eventdb.Event
	header := genesisBlock.Header()
	for _, e := range blockEvents {
		events = append(events, eventdb.NewEvent(header, 0, thor.Bytes32{}, thor.Address{}, e))
	}
	eventDB.Insert(events, nil)

	chain, err := chain.New(lvldb, genesisBlock)
	if err != nil {
		return nil, err
	}

	proposer, privateKey, err := loadProposer(ctx.Bool("devnet"), dataDir+"/master.key")
	if err != nil {
		return nil, err
	}
	log.Info("Proposer key loaded", "address", proposer)

	beneficiary := proposer
	if ctx.String("beneficiary") != "" {
		if beneficiary, err = thor.ParseAddress(ctx.String("beneficiary")); err != nil {
			return nil, err
		}
	}
	log.Info("Beneficiary key loaded", "address", beneficiary)

	p2p, err := initP2PSrv(ctx, dataDir)
	if err != nil {
		return nil, err
	}

	txpool := txpool.New(chain, stateCreator)
	communicator := comm.New(chain, txpool)

	api := api.New(chain, stateCreator, txpool, eventDB, transferDB, communicator)
	var handleAPI http.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
		if domains := ctx.String("apicors"); domains != "" {
			w.Header().Set("Access-Control-Allow-Origin", domains)
		}
		api(w, req)
	}

	return &components{
		chain:        chain,
		txpool:       txpool,
		p2p:          p2p,
		communicator: communicator,
		consensus:    consensus.New(chain, stateCreator),
		packer:       &packer{Packer.New(chain, stateCreator, proposer, beneficiary), privateKey},
		apiSrv:       &http.Server{Handler: handleAPI},
	}, nil
}

func initP2PSrv(ctx *cli.Context, dataDir string) (*p2psrv.Server, error) {
	nodeKey, err := loadKey(dataDir + "/node.key")
	if err != nil {
		return nil, err
	}

	opt := &p2psrv.Options{
		PrivateKey:     nodeKey,
		MaxPeers:       ctx.Int("maxpeers"),
		ListenAddr:     fmt.Sprintf(":%v", ctx.Int("p2pport")),
		BootstrapNodes: []*discover.Node{discover.MustParseNode(boot)},
	}
	var nodes p2psrv.Nodes
	nodesByte, err := ioutil.ReadFile(dataDir + "/nodes.cache")
	if err != nil {
		if !os.IsNotExist(err) {
			log.Error(fmt.Sprintf("%v", err))
		}
	} else {
		rlp.DecodeBytes(nodesByte, &nodes)
		opt.GoodNodes = nodes
	}

	log.Info("Thor network initialized", "listen-addr", opt.ListenAddr, "max-peers", opt.MaxPeers, "node-key-address", crypto.PubkeyToAddress(nodeKey.PublicKey))
	return p2psrv.New(opt), nil
}

// copy from go-ethereum
func defaultMainDir() string {
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
