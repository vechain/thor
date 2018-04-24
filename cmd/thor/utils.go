package main

import (
	"crypto/ecdsa"
	"fmt"
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
	Genesis "github.com/vechain/thor/genesis"
	Logdb "github.com/vechain/thor/logdb"
	Lvldb "github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	Packer "github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Transferdb "github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

func initLog(lvl log15.Lvl) {
	log15.Root().SetHandler(log15.LvlFilterHandler(lvl, log15.StderrHandler))
	// set go-ethereum log lvl to Warn
	ethLogHandler := ethlog.NewGlogHandler(ethlog.StreamHandler(os.Stderr, ethlog.TerminalFormat(true)))
	ethLogHandler.Verbosity(ethlog.LvlWarn)
	ethlog.Root().SetHandler(ethLogHandler)
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

func genesis(isDev bool) (*Genesis.Genesis, error) {
	var (
		genesis *Genesis.Genesis
		err     error
	)
	if isDev {
		genesis, err = Genesis.NewDevnet()
		log.Info("Using Devnet", "genesis", genesis.ID().AbbrevString())
	} else {
		genesis, err = Genesis.NewMainnet()
		log.Info("Using Mainnet", "genesis", genesis.ID().AbbrevString())
	}

	return genesis, err
}

func dataDir(genesis *Genesis.Genesis, root string) (string, error) {
	dataDir := fmt.Sprintf("%v/chain-%x", root, genesis.ID().Bytes()[24:])
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
	}
	log.Info("Disk storage enabled for storing data", "path", dataDir)

	return dataDir, nil
}

func makeComponent(
	ctx *cli.Context,
	lvldb *Lvldb.LevelDB,
	logdb *Logdb.LogDB,
	transferDB *Transferdb.TransferDB,
	genesis *Genesis.Genesis,
	dataDir string,
) (*components, error) {
	stateCreator := state.NewCreator(lvldb)

	genesisBlock, txLogs, err := genesis.Build(stateCreator)
	if err != nil {
		return nil, err
	}

	logs := []*Logdb.Log{}
	header := genesisBlock.Header()
	for _, log := range txLogs {
		logs = append(logs, Logdb.NewLog(header, 0, thor.Bytes32{}, thor.Address{}, log))
	}
	logdb.Insert(logs, nil)

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

	return &components{
		chain:        chain,
		txpool:       txpool,
		p2p:          p2p,
		communicator: communicator,
		consensus:    consensus.New(chain, stateCreator),
		packer:       &packer{Packer.New(chain, stateCreator, proposer, beneficiary), privateKey},
		apiSrv:       &http.Server{Handler: api.New(chain, stateCreator, txpool, logdb, transferDB, communicator)},
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
			return filepath.Join(home, "Library", "VeChain", "thor")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "VeChain", "thor")
		} else {
			return filepath.Join(home, ".vechain", "thor")
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
