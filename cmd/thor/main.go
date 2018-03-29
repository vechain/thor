package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/vechain/thor/cmd/thor/app"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
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

func main() {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Thor"
	app.Usage = "Core of VeChain"
	app.Copyright = "2018 VeChain Foundation <https://vechain.org/>"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "port",
			Value: ":56565",
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

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func action(ctx *cli.Context) error {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
	glogger.Verbosity(log.Lvl(ctx.Int("verbosity")))
	glogger.Vmodule(ctx.String("vmodule"))
	log.Root().SetHandler(glogger)

	lv, err := lvldb.New(ctx.String("datadir"), lvldb.Options{})
	if err != nil {
		return err
	}
	defer lv.Close()

	logdb, err := logdb.New(ctx.String("datadir") + "/log.db")
	if err != nil {
		return err
	}
	defer logdb.Close()

	nodeKey, err := loadNodeKey(ctx)
	if err != nil {
		return err
	}

	proposer, privateKey, err := loadAccount(ctx)
	if err != nil {
		return err
	}

	app, err := app.New(lv, proposer, logdb, nodeKey, ctx.String("port"))
	if err != nil {
		return err
	}

	var goes co.Goes
	c, cancel := context.WithCancel(context.Background())
	goes.Go(func() {
		app.Run(c, ctx.String("restfulport"), privateKey)
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
