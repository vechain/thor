package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	version   = "1.0"
	gitCommit string
	release   = "dev"
)

// Options for Client.
type Options struct {
	DataPath    string
	Bind        string
	Proposer    thor.Address
	Beneficiary thor.Address
	PrivateKey  *ecdsa.PrivateKey
}

func newApp() *cli.App {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Thor"
	app.Usage = "Core of VeChain"
	app.Copyright = "2018 VeChain Foundation <https://vechain.org/>"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "port",
			Value: ":55555",
			Usage: "p2p listen port",
		},
		cli.StringFlag{
			Name:  "restfulport",
			Value: ":8081",
			Usage: "restful port",
		},
		cli.StringFlag{
			Name:  "nodekey",
			Usage: "private key (for node) file path (defaults to ~/.thor-node.key if omitted)",
		},
		cli.StringFlag{
			Name:  "key",
			Usage: "private key (for pack) as hex (for testing)",
		},
		cli.StringFlag{
			Name:  "datadir",
			Value: "/tmp/thor_datadir_test",
			Usage: "chain data path",
		},
		cli.IntFlag{
			Name:  "verbosity",
			Value: int(log.LvlInfo),
			Usage: "log verbosity (0-9)",
		},
		cli.StringFlag{
			Name:  "vmodule",
			Usage: "log verbosity pattern",
		},
	}
	app.Action = action

	return app
}

func action(ctx *cli.Context) error {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
	glogger.Verbosity(log.Lvl(ctx.Int("verbosity")))
	glogger.Vmodule(ctx.String("vmodule"))
	log.Root().SetHandler(glogger)

	nodeKey, err := loadNodeKey(ctx)
	if err != nil {
		return err
	}

	proposer, privateKey, err := loadAccount(ctx)
	if err != nil {
		return err
	}

	lv, err := lvldb.New(ctx.String("datadir"), lvldb.Options{})
	if err != nil {
		return err
	}
	defer lv.Close()

	ldb, err := logdb.New(ctx.String("datadir") + "/log.db")
	if err != nil {
		return err
	}
	defer ldb.Close()

	stateCreator := state.NewCreator(lv)

	genesisBlock, _, err := genesis.Dev.Build(stateCreator)
	if err != nil {
		return err
	}

	ch := chain.New(lv)
	if err := ch.WriteGenesis(genesisBlock); err != nil {
		return err
	}

	peerCh := make(chan *p2psrv.Peer)
	srv := p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     nodeKey,
			MaxPeers:       25,
			ListenAddr:     ctx.String("port"),
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})
	srv.SubscribePeer(peerCh)

	txpool := txpool.New()

	cm := comm.New(ch, txpool, stateCreator)

	srv.Start("thor@111111", cm.Protocols())
	defer srv.Stop()

	cm.Start(peerCh)
	defer cm.Stop()

	lsr, err := net.Listen("tcp", ctx.String("restfulport"))
	if err != nil {
		return err
	}
	defer lsr.Close()

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())

	goes.Go(func() {
		txCh := make(chan *tx.Transaction)
		sub := txpool.SubscribeNewTransaction(txCh)

		select {
		case <-c.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			cm.BroadcastTx(tx)
		}
	})

	goes.Go(func() {
		txCh := make(chan *tx.Transaction)
		sub := cm.SubscribeTx(txCh)

		select {
		case <-c.Done():
			sub.Unsubscribe()
			return
		case tx := <-txCh:
			txpool.Add(tx)
		}
	})

	packedChan := make(chan packedEvent)
	bestBlockUpdate := make(chan struct{})

	goes.Go(func() {
		consent := newConsent(cm, ch, stateCreator)
		consent.subscribeBestBlockUpdate(bestBlockUpdate)
		consent.run(c, packedChan)
	})

	goes.Go(func() {
		pk := packer.New(ch, stateCreator, proposer, proposer)
		timer := time.NewTimer(1 * time.Second)
		defer timer.Stop()

		for {
			timer.Reset(1 * time.Second)

			select {
			case <-c.Done():
				return
			case <-bestBlockUpdate:
				break
			case <-timer.C:
				if cm.IsSynced() {
					txIter, err := txpool.NewIterator(ch, stateCreator)
					if err != nil {
						log.Warn(fmt.Sprintf("%v", err))
						continue
					}
					pack(c, ch, pk, txIter, privateKey, packedChan, bestBlockUpdate)
				} else {
					log.Warn("has not synced")
				}
			}
		}
	})

	goes.Go(func() {
		restful := http.Server{Handler: api.NewHTTPHandler(ch, stateCreator, txpool, ldb)}

		go func() {
			<-c.Done()
			restful.Shutdown(context.TODO())
		}()

		if err := restful.Serve(lsr); err != http.ErrServerClosed {
			log.Error(fmt.Sprintf("%v", err))
		}
	})

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	select {
	case <-interrupt:
		cancel()
		goes.Wait()
	}

	return nil
}

func loadNodeKey(ctx *cli.Context) (key *ecdsa.PrivateKey, err error) {
	keyFile := ctx.String("nodekey")
	if keyFile == "" {
		// no file specified, use default file path
		home, err := homeDir()
		if err != nil {
			return nil, err
		}
		keyFile = filepath.Join(home, ".thor-node.key")
	} else if !filepath.IsAbs(keyFile) {
		// resolve to absolute path
		keyFile, err = filepath.Abs(keyFile)
		if err != nil {
			return nil, err
		}
	}

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

func loadAccount(ctx *cli.Context) (thor.Address, *ecdsa.PrivateKey, error) {
	keyString := ctx.String("key")
	if keyString != "" {
		key, err := crypto.HexToECDSA(keyString)
		if err != nil {
			return thor.Address{}, nil, err
		}
		return thor.Address(crypto.PubkeyToAddress(key.PublicKey)), key, nil
	}

	index := rand.Intn(len(genesis.Dev.Accounts()))
	return genesis.Dev.Accounts()[index].Address, genesis.Dev.Accounts()[index].PrivateKey, nil
}

func homeDir() (string, error) {
	// try to get HOME env
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	user, err := user.Current()
	if err != nil {
		return "", err
	}
	if user.HomeDir != "" {
		return user.HomeDir, nil
	}

	return os.Getwd()
}

func pack(
	ctx context.Context,
	ch *chain.Chain,
	pk *packer.Packer,
	txIter *txpool.Iterator,
	privateKey *ecdsa.PrivateKey,
	packedChan chan packedEvent,
	bestBlockUpdate chan struct{}) {

	bestBlock, err := ch.GetBestBlock()
	if err != nil {
		return
	}

	now := uint64(time.Now().Unix())
	if ts, adopt, commit, err := pk.Prepare(bestBlock.Header(), now); err == nil {
		waitSec := ts - now
		log.Info(fmt.Sprintf("waiting to propose new block(#%v)", bestBlock.Header().Number()+1), "after", fmt.Sprintf("%vs", waitSec))

		waitTime := time.NewTimer(time.Duration(waitSec) * time.Second)
		defer waitTime.Stop()

		select {
		case <-waitTime.C:
			for txIter.HasNext() {
				err := adopt(txIter.Next())
				if packer.IsGasLimitReached(err) {
					break
				}
			}

			if blk, _, err := commit(privateKey); err == nil {
				log.Info(fmt.Sprintf("proposed new block(#%v)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
				pe := packedEvent{blk: blk, ack: make(chan struct{})}
				packedChan <- pe
				<-pe.ack
			}
		case <-bestBlockUpdate:
			return
		case <-ctx.Done():
			return
		}
	}
}
