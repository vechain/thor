// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/inconshreveable/log15"
	isatty "github.com/mattn/go-isatty"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/bft"
	"github.com/vechain/thor/cmd/thor/node"
	"github.com/vechain/thor/cmd/thor/optimizer"
	"github.com/vechain/thor/cmd/thor/solo"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"

	// Force-load the tracer engines to trigger registration
	_ "github.com/vechain/thor/tracers/native"
)

var (
	version   string
	gitCommit string
	gitTag    string
	log       = log15.New()

	defaultTxPoolOptions = txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     20 * time.Minute,
	}
)

func fullVersion() string {
	versionMeta := "release"
	if gitTag == "" {
		versionMeta = "dev"
	}
	return fmt.Sprintf("%s-%s-%s", version, gitCommit, versionMeta)
}

func main() {
	app := cli.App{
		Version:   fullVersion(),
		Name:      "Thor",
		Usage:     "Node of VeChain Thor Network",
		Copyright: "2018 VeChain Foundation <https://vechain.org/>",
		Flags: []cli.Flag{
			networkFlag,
			configDirFlag,
			dataDirFlag,
			cacheFlag,
			beneficiaryFlag,
			targetGasLimitFlag,
			apiAddrFlag,
			apiCorsFlag,
			apiTimeoutFlag,
			apiCallGasLimitFlag,
			apiBacktraceLimitFlag,
			verbosityFlag,
			maxPeersFlag,
			p2pPortFlag,
			natFlag,
			bootNodeFlag,
			skipLogsFlag,
			pprofFlag,
			verifyLogsFlag,
			disablePrunerFlag,
		},
		Action: defaultAction,
		Commands: []cli.Command{
			{
				Name:  "solo",
				Usage: "client runs in solo mode for test & dev",
				Flags: []cli.Flag{
					dataDirFlag,
					cacheFlag,
					apiAddrFlag,
					apiCorsFlag,
					apiTimeoutFlag,
					apiCallGasLimitFlag,
					apiBacktraceLimitFlag,
					onDemandFlag,
					persistFlag,
					gasLimitFlag,
					verbosityFlag,
					pprofFlag,
					verifyLogsFlag,
					skipLogsFlag,
					txPoolLimitFlag,
					txPoolLimitPerAccountFlag,
					disablePrunerFlag,
				},
				Action: soloAction,
			},
			{
				Name:  "master-key",
				Usage: "master key management",
				Flags: []cli.Flag{
					configDirFlag,
					importMasterKeyFlag,
					exportMasterKeyFlag,
				},
				Action: masterKeyAction,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultAction(ctx *cli.Context) error {
	exitSignal := handleExitSignal()

	defer func() { log.Info("exited") }()

	initLogger(ctx)
	gene, forkConfig, err := selectGenesis(ctx)
	if err != nil {
		return err
	}
	instanceDir, err := makeInstanceDir(ctx, gene)
	if err != nil {
		return err
	}

	mainDB, err := openMainDB(ctx, instanceDir)
	if err != nil {
		return err
	}
	defer func() { log.Info("closing main database..."); mainDB.Close() }()

	skipLogs := ctx.Bool(skipLogsFlag.Name)

	logDB, err := openLogDB(ctx, instanceDir)
	if err != nil {
		return err
	}
	defer func() { log.Info("closing log database..."); logDB.Close() }()

	repo, err := initChainRepository(gene, mainDB, logDB)
	if err != nil {
		return err
	}

	master, err := loadNodeMaster(ctx)
	if err != nil {
		return err
	}

	printStartupMessage1(gene, repo, master, instanceDir, forkConfig)

	if !skipLogs {
		if err := syncLogDB(exitSignal, repo, logDB, ctx.Bool(verifyLogsFlag.Name)); err != nil {
			return err
		}
	}

	txpoolOpt := defaultTxPoolOptions
	txPool := txpool.New(repo, state.NewStater(mainDB), txpoolOpt)
	defer func() { log.Info("closing tx pool..."); txPool.Close() }()

	p2pcom, err := newP2PComm(ctx, repo, txPool, instanceDir)
	if err != nil {
		return err
	}

	bftEngine, err := bft.NewEngine(repo, mainDB, forkConfig, master.Address())
	if err != nil {
		return errors.Wrap(err, "init bft engine")
	}

	apiHandler, apiCloser := api.New(
		repo,
		state.NewStater(mainDB),
		txPool,
		logDB,
		bftEngine,
		p2pcom.comm,
		ctx.String(apiCorsFlag.Name),
		uint32(ctx.Int(apiBacktraceLimitFlag.Name)),
		uint64(ctx.Int(apiCallGasLimitFlag.Name)),
		ctx.Bool(pprofFlag.Name),
		skipLogs,
		forkConfig)
	defer func() { log.Info("closing API..."); apiCloser() }()

	apiURL, srvCloser, err := startAPIServer(ctx, apiHandler, repo.GenesisBlock().Header().ID())
	if err != nil {
		return err
	}
	defer func() { log.Info("stopping API server..."); srvCloser() }()

	printStartupMessage2(apiURL, p2pcom.enode)

	if err := p2pcom.Start(); err != nil {
		return err
	}
	defer p2pcom.Stop()

	optimizer := optimizer.New(mainDB, repo, !ctx.Bool(disablePrunerFlag.Name))
	defer func() { log.Info("stopping optimizer..."); optimizer.Stop() }()

	return node.New(
		master,
		repo,
		bftEngine,
		state.NewStater(mainDB),
		logDB,
		txPool,
		filepath.Join(instanceDir, "tx.stash"),
		p2pcom.comm,
		uint64(ctx.Int(targetGasLimitFlag.Name)),
		skipLogs,
		forkConfig).Run(exitSignal)
}

func soloAction(ctx *cli.Context) error {
	exitSignal := handleExitSignal()
	defer func() { log.Info("exited") }()

	initLogger(ctx)
	gene := genesis.NewDevnet()
	// Solo forks from the start
	forkConfig := thor.ForkConfig{}

	var mainDB *muxdb.MuxDB
	var logDB *logdb.LogDB
	var instanceDir string
	var err error

	if ctx.Bool(persistFlag.Name) {
		if instanceDir, err = makeInstanceDir(ctx, gene); err != nil {
			return err
		}
		if mainDB, err = openMainDB(ctx, instanceDir); err != nil {
			return err
		}
		defer func() { log.Info("closing main database..."); mainDB.Close() }()
		if logDB, err = openLogDB(ctx, instanceDir); err != nil {
			return err
		}
		defer func() { log.Info("closing log database..."); logDB.Close() }()
	} else {
		instanceDir = "Memory"
		mainDB = openMemMainDB()
		logDB = openMemLogDB()
	}

	repo, err := initChainRepository(gene, mainDB, logDB)
	if err != nil {
		return err
	}

	skipLogs := ctx.Bool(skipLogsFlag.Name)

	if !skipLogs {
		if err := syncLogDB(exitSignal, repo, logDB, ctx.Bool(verifyLogsFlag.Name)); err != nil {
			return err
		}
	}

	txPoolOption := defaultTxPoolOptions
	txPoolOption.Limit = ctx.Int(txPoolLimitFlag.Name)
	txPoolOption.LimitPerAccount = ctx.Int(txPoolLimitPerAccountFlag.Name)

	txPool := txpool.New(repo, state.NewStater(mainDB), txPoolOption)
	defer func() { log.Info("closing tx pool..."); txPool.Close() }()

	bftEngine := solo.NewBFTEngine(repo)
	apiHandler, apiCloser := api.New(
		repo,
		state.NewStater(mainDB),
		txPool,
		logDB,
		bftEngine,
		&solo.Communicator{},
		ctx.String(apiCorsFlag.Name),
		uint32(ctx.Int(apiBacktraceLimitFlag.Name)),
		uint64(ctx.Int(apiCallGasLimitFlag.Name)),
		ctx.Bool(pprofFlag.Name),
		skipLogs,
		forkConfig)
	defer func() { log.Info("closing API..."); apiCloser() }()

	apiURL, srvCloser, err := startAPIServer(ctx, apiHandler, repo.GenesisBlock().Header().ID())
	if err != nil {
		return err
	}
	defer func() { log.Info("stopping API server..."); srvCloser() }()

	printSoloStartupMessage(gene, repo, instanceDir, apiURL, forkConfig)

	optimizer := optimizer.New(mainDB, repo, !ctx.Bool(disablePrunerFlag.Name))
	defer func() { log.Info("stopping optimizer..."); optimizer.Stop() }()

	return solo.New(repo,
		state.NewStater(mainDB),
		logDB,
		txPool,
		uint64(ctx.Int(gasLimitFlag.Name)),
		ctx.Bool(onDemandFlag.Name),
		skipLogs,
		forkConfig).Run(exitSignal)
}

func masterKeyAction(ctx *cli.Context) error {
	hasImportFlag := ctx.Bool(importMasterKeyFlag.Name)
	hasExportFlag := ctx.Bool(exportMasterKeyFlag.Name)
	if hasImportFlag && hasExportFlag {
		return fmt.Errorf("flag %s and %s are exclusive", importMasterKeyFlag.Name, exportMasterKeyFlag.Name)
	}

	keyPath, err := masterKeyPath(ctx)
	if err != nil {
		return err
	}

	if !hasImportFlag && !hasExportFlag {
		masterKey, err := loadOrGeneratePrivateKey(keyPath)
		if err != nil {
			return err
		}
		fmt.Println("Master:", thor.Address(crypto.PubkeyToAddress(masterKey.PublicKey)))
		return nil
	}

	if hasImportFlag {
		if isatty.IsTerminal(os.Stdin.Fd()) {
			fmt.Println("Input JSON keystore (end with ^d):")
		}
		keyjson, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(keyjson, &map[string]interface{}{}); err != nil {
			return errors.WithMessage(err, "unmarshal")
		}
		password, err := readPasswordFromNewTTY("Enter passphrase: ")
		if err != nil {
			return err
		}

		key, err := keystore.DecryptKey(keyjson, password)
		if err != nil {
			return errors.WithMessage(err, "decrypt")
		}

		if err := crypto.SaveECDSA(keyPath, key.PrivateKey); err != nil {
			return err
		}
		fmt.Println("Master key imported:", thor.Address(key.Address))
		return nil
	}

	if hasExportFlag {
		masterKey, err := loadOrGeneratePrivateKey(keyPath)
		if err != nil {
			return err
		}

		password, err := readPasswordFromNewTTY("Enter passphrase: ")
		if err != nil {
			return err
		}
		if password == "" {
			return errors.New("non-empty passphrase required")
		}
		confirm, err := readPasswordFromNewTTY("Confirm passphrase: ")
		if err != nil {
			return err
		}

		if password != confirm {
			return errors.New("passphrase confirmation mismatch")
		}

		keyjson, err := keystore.EncryptKey(&keystore.Key{
			PrivateKey: masterKey,
			Address:    crypto.PubkeyToAddress(masterKey.PublicKey),
			Id:         uuid.NewRandom()},
			password, keystore.StandardScryptN, keystore.StandardScryptP)
		if err != nil {
			return err
		}
		if isatty.IsTerminal(os.Stdout.Fd()) {
			fmt.Println("=== JSON keystore ===")
		}
		_, err = fmt.Println(string(keyjson))
		return err
	}
	return nil
}
