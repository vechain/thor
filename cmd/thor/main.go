// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mattn/go-isatty"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cmd/thor/node"
	"github.com/vechain/thor/v2/cmd/thor/pruner"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
	"gopkg.in/urfave/cli.v1"

	// Force-load the tracer engines to trigger registration
	_ "github.com/vechain/thor/v2/tracers/js"
	_ "github.com/vechain/thor/v2/tracers/logger"
	_ "github.com/vechain/thor/v2/tracers/native"
)

var (
	version              string
	gitCommit            string
	gitTag               string
	copyrightYear        string
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
		Copyright: fmt.Sprintf("2018-%s VeChain Foundation <https://vechain.org/>", copyrightYear),
		Flags: []cli.Flag{
			networkFlag,
			configDirFlag,
			masterKeyStdinFlag,
			dataDirFlag,
			cacheFlag,
			beneficiaryFlag,
			targetGasLimitFlag,
			apiAddrFlag,
			apiCorsFlag,
			apiTimeoutFlag,
			apiCallGasLimitFlag,
			apiBacktraceLimitFlag,
			apiAllowCustomTracerFlag,
			apiEnableDeprecatedFlag,
			enableAPILogsFlag,
			apiLogsLimitFlag,
			verbosityFlag,
			jsonLogsFlag,
			maxPeersFlag,
			p2pPortFlag,
			natFlag,
			bootNodeFlag,
			allowedPeersFlag,
			skipLogsFlag,
			pprofFlag,
			verifyLogsFlag,
			disablePrunerFlag,
			enableMetricsFlag,
			metricsAddrFlag,
			adminAddrFlag,
			enableAdminFlag,
			txPoolLimitPerAccountFlag,
			allowedTracersFlag,
		},
		Action: defaultAction,
		Commands: []cli.Command{
			{
				Name:  "solo",
				Usage: "client runs in solo mode for test & dev",
				Flags: []cli.Flag{
					genesisFlag,
					dataDirFlag,
					cacheFlag,
					apiAddrFlag,
					apiCorsFlag,
					apiTimeoutFlag,
					apiCallGasLimitFlag,
					apiBacktraceLimitFlag,
					apiAllowCustomTracerFlag,
					apiEnableDeprecatedFlag,
					enableAPILogsFlag,
					apiLogsLimitFlag,
					onDemandFlag,
					blockInterval,
					persistFlag,
					gasLimitFlag,
					verbosityFlag,
					jsonLogsFlag,
					pprofFlag,
					verifyLogsFlag,
					skipLogsFlag,
					txPoolLimitFlag,
					txPoolLimitPerAccountFlag,
					disablePrunerFlag,
					enableMetricsFlag,
					metricsAddrFlag,
					adminAddrFlag,
					enableAdminFlag,
					allowedTracersFlag,
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

	lvl, err := readIntFromUInt64Flag(ctx.Uint64(verbosityFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse verbosity flag")
	}
	logLevel := initLogger(lvl, ctx.Bool(jsonLogsFlag.Name))

	// enable metrics as soon as possible
	enableMetrics := ctx.Bool(enableMetricsFlag.Name)
	metricsURL := ""
	if enableMetrics {
		metrics.InitializePrometheusMetrics()
		url, closeFunc, err := api.StartMetricsServer(ctx.String(metricsAddrFlag.Name))
		if err != nil {
			return fmt.Errorf("unable to start metrics server - %w", err)
		}
		metricsURL = url
		defer func() { log.Info("stopping metrics server..."); closeFunc() }()
	}

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
	if enableMetrics {
		mainDB.EnableMetrics()
	}
	defer func() { log.Info("closing main database..."); mainDB.Close() }()

	logDB, err := openLogDB(instanceDir)
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

	skipLogs := ctx.Bool(skipLogsFlag.Name)
	if !skipLogs {
		if err := syncLogDB(exitSignal, repo, logDB, ctx.Bool(verifyLogsFlag.Name)); err != nil {
			return err
		}
	}

	txpoolOpt := defaultTxPoolOptions
	txpoolOpt.LimitPerAccount, err = readIntFromUInt64Flag(ctx.Uint64(txPoolLimitPerAccountFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse txpool-limit-per-account flag")
	}
	txPool := txpool.New(repo, state.NewStater(mainDB), txpoolOpt)
	defer func() { log.Info("closing tx pool..."); txPool.Close() }()

	p2pCommunicator, err := newP2PCommunicator(ctx, repo, txPool, instanceDir)
	if err != nil {
		return err
	}

	adminURL := ""
	logAPIRequests := &atomic.Bool{}
	logAPIRequests.Store(ctx.Bool(enableAPILogsFlag.Name))
	if ctx.Bool(enableAdminFlag.Name) {
		url, closeFunc, err := api.StartAdminServer(
			ctx.String(adminAddrFlag.Name),
			logLevel,
			repo,
			p2pCommunicator.Communicator(),
			logAPIRequests,
		)
		if err != nil {
			return fmt.Errorf("unable to start admin server - %w", err)
		}
		adminURL = url
		defer func() { log.Info("stopping admin server..."); closeFunc() }()
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
		p2pCommunicator.Communicator(),
		forkConfig,
		makeAPIConfig(ctx, logAPIRequests, false),
	)
	defer func() { log.Info("closing API..."); apiCloser() }()

	apiURL, srvCloser, err := startAPIServer(ctx, apiHandler, repo.GenesisBlock().Header().ID())
	if err != nil {
		return err
	}
	defer func() { log.Info("stopping API server..."); srvCloser() }()

	printStartupMessage2(gene, apiURL, p2pCommunicator.Enode(), metricsURL, adminURL)

	if err := p2pCommunicator.Start(); err != nil {
		return err
	}
	defer p2pCommunicator.Stop()

	if !ctx.Bool(disablePrunerFlag.Name) {
		pruner := pruner.New(mainDB, repo)
		defer func() { log.Info("stopping pruner..."); pruner.Stop() }()
	}

	return node.New(
		master,
		repo,
		bftEngine,
		state.NewStater(mainDB),
		logDB,
		txPool,
		filepath.Join(instanceDir, "tx.stash"),
		p2pCommunicator.Communicator(),
		ctx.Uint64(targetGasLimitFlag.Name),
		skipLogs,
		forkConfig,
	).Run(exitSignal)
}

func soloAction(ctx *cli.Context) error {
	exitSignal := handleExitSignal()
	defer func() { log.Info("exited") }()

	lvl, err := readIntFromUInt64Flag(ctx.Uint64(verbosityFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse verbosity flag")
	}

	logLevel := initLogger(lvl, ctx.Bool(jsonLogsFlag.Name))

	onDemandBlockProduction := ctx.Bool(onDemandFlag.Name)
	blockProductionInterval := ctx.Uint64(blockInterval.Name)
	if blockProductionInterval == 0 {
		return errors.New("block-interval cannot be zero")
	}

	// enable metrics as soon as possible
	enableMetrics := ctx.Bool(enableMetricsFlag.Name)
	metricsURL := ""
	if enableMetrics {
		metrics.InitializePrometheusMetrics()
		url, closeFunc, err := api.StartMetricsServer(ctx.String(metricsAddrFlag.Name))
		if err != nil {
			return fmt.Errorf("unable to start metrics server - %w", err)
		}
		metricsURL = url
		defer func() { log.Info("stopping metrics server..."); closeFunc() }()
	}

	var (
		gene       *genesis.Genesis
		forkConfig thor.ForkConfig
	)

	flagGenesis := ctx.String(genesisFlag.Name)
	if flagGenesis == "" {
		gene = genesis.NewDevnet()
		forkConfig = thor.SoloFork
	} else {
		gene, forkConfig, err = parseGenesisFile(flagGenesis)
		if err != nil {
			return err
		}
	}

	var mainDB *muxdb.MuxDB
	var logDB *logdb.LogDB
	var instanceDir string

	if ctx.Bool(persistFlag.Name) {
		if instanceDir, err = makeInstanceDir(ctx, gene); err != nil {
			return err
		}
		if mainDB, err = openMainDB(ctx, instanceDir); err != nil {
			return err
		}
		if enableMetrics {
			mainDB.EnableMetrics()
		}
		defer func() { log.Info("closing main database..."); mainDB.Close() }()

		if logDB, err = openLogDB(instanceDir); err != nil {
			return err
		}
		defer func() { log.Info("closing log database..."); logDB.Close() }()
	} else {
		instanceDir = "Memory"
		mainDB = openMemMainDB() // Skip metrics of in-memory DB
		logDB = openMemLogDB()
	}

	repo, err := initChainRepository(gene, mainDB, logDB)
	if err != nil {
		return err
	}

	adminURL := ""
	logAPIRequests := &atomic.Bool{}
	logAPIRequests.Store(ctx.Bool(enableAPILogsFlag.Name))
	if ctx.Bool(enableAdminFlag.Name) {
		url, closeFunc, err := api.StartAdminServer(
			ctx.String(adminAddrFlag.Name),
			logLevel,
			repo,
			nil,
			logAPIRequests,
		)
		if err != nil {
			return fmt.Errorf("unable to start admin server - %w", err)
		}
		adminURL = url
		defer func() { log.Info("stopping admin server..."); closeFunc() }()
	}

	printStartupMessage1(gene, repo, nil, instanceDir, forkConfig)

	skipLogs := ctx.Bool(skipLogsFlag.Name)
	if !skipLogs {
		if err := syncLogDB(exitSignal, repo, logDB, ctx.Bool(verifyLogsFlag.Name)); err != nil {
			return err
		}
	}

	txPoolOption := defaultTxPoolOptions
	txPoolOption.Limit, err = readIntFromUInt64Flag(ctx.Uint64(txPoolLimitFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse txpool-limit flag")
	}
	txPoolOption.LimitPerAccount, err = readIntFromUInt64Flag(ctx.Uint64(txPoolLimitPerAccountFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse txpool-limit-per-account flag")
	}

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
		forkConfig,
		makeAPIConfig(ctx, logAPIRequests, true),
	)
	defer func() { log.Info("closing API..."); apiCloser() }()

	apiURL, srvCloser, err := startAPIServer(ctx, apiHandler, repo.GenesisBlock().Header().ID())
	if err != nil {
		return err
	}
	defer func() {
		log.Info("stopping API server...")
		srvCloser()
	}()

	blockInterval := ctx.Uint64(blockInterval.Name)
	if blockInterval == 0 {
		return errors.New("block-interval cannot be zero")
	}

	printStartupMessage2(gene, apiURL, "", metricsURL, adminURL)

	if !ctx.Bool(disablePrunerFlag.Name) {
		pruner := pruner.New(mainDB, repo)
		defer func() { log.Info("stopping pruner..."); pruner.Stop() }()
	}

	return solo.New(repo,
		state.NewStater(mainDB),
		logDB,
		txPool,
		ctx.Uint64(gasLimitFlag.Name),
		onDemandBlockProduction,
		skipLogs,
		blockProductionInterval,
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
		keyjson, err := io.ReadAll(os.Stdin)
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
