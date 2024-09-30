// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"github.com/vechain/thor/v2/log"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	networkFlag = cli.StringFlag{
		Name:  "network",
		Usage: "the network to join (main|test) or path to genesis file",
	}
	configDirFlag = cli.StringFlag{
		Name:   "config-dir",
		Value:  defaultConfigDir(),
		Hidden: true,
		Usage:  "directory for user global configurations",
	}
	masterKeyStdinFlag = cli.BoolFlag{
		Name:   "master-key-stdin",
		Usage:  "read master key from stdin",
		Hidden: true,
	}
	dataDirFlag = cli.StringFlag{
		Name:  "data-dir",
		Value: defaultDataDir(),
		Usage: "directory for block-chain databases",
	}
	beneficiaryFlag = cli.StringFlag{
		Name:  "beneficiary",
		Usage: "address for block rewards",
	}
	apiAddrFlag = cli.StringFlag{
		Name:  "api-addr",
		Value: "localhost:8669",
		Usage: "API service listening address",
	}
	apiCorsFlag = cli.StringFlag{
		Name:  "api-cors",
		Value: "",
		Usage: "comma separated list of domains from which to accept cross origin requests to API",
	}
	apiTimeoutFlag = cli.Uint64Flag{
		Name:  "api-timeout",
		Value: 10000,
		Usage: "API request timeout value in milliseconds",
	}
	apiCallGasLimitFlag = cli.Uint64Flag{
		Name:  "api-call-gas-limit",
		Value: 50000000,
		Usage: "limit contract call gas",
	}
	apiBacktraceLimitFlag = cli.Uint64Flag{
		Name:  "api-backtrace-limit",
		Value: 1000,
		Usage: "limit the distance between 'position' and best block for subscriptions APIs",
	}
	apiAllowCustomTracerFlag = cli.BoolFlag{
		Name:  "api-allow-custom-tracer",
		Usage: "allow custom JS tracer to be used tracer API",
	}
	apiLogsLimitFlag = cli.Uint64Flag{
		Name:  "api-logs-limit",
		Value: 1000,
		Usage: "limit the number of logs returned by /logs API",
	}
	enableAPILogsFlag = cli.BoolFlag{
		Name:  "enable-api-logs",
		Usage: "enables API requests logging",
	}
	verbosityFlag = cli.Uint64Flag{
		Name:  "verbosity",
		Value: log.LegacyLevelInfo,
		Usage: "log verbosity (0-9)",
	}
	jsonLogsFlag = cli.BoolFlag{
		Name:  "json-logs",
		Usage: "output logs in JSON format",
	}
	maxPeersFlag = cli.Uint64Flag{
		Name:  "max-peers",
		Usage: "maximum number of P2P network peers (P2P network disabled if set to 0)",
		Value: 25,
	}
	p2pPortFlag = cli.Uint64Flag{
		Name:  "p2p-port",
		Value: 11235,
		Usage: "P2P network listening port",
	}
	natFlag = cli.StringFlag{
		Name:  "nat",
		Value: "any",
		Usage: "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
	}
	bootNodeFlag = cli.StringFlag{
		Name:  "bootnode",
		Usage: "comma separated list of bootstrap node IDs",
	}
	allowedPeersFlag = cli.StringFlag{
		Name:   "allowed-peers",
		Hidden: true,
		Usage:  "comma separated list of node IDs that can be connected to",
	}
	importMasterKeyFlag = cli.BoolFlag{
		Name:  "import",
		Usage: "import master key from keystore",
	}
	exportMasterKeyFlag = cli.BoolFlag{
		Name:  "export",
		Usage: "export master key to keystore",
	}
	targetGasLimitFlag = cli.Uint64Flag{
		Name:  "target-gas-limit",
		Value: 0,
		Usage: "target block gas limit (adaptive if set to 0)",
	}
	pprofFlag = cli.BoolFlag{
		Name:  "pprof",
		Usage: "turn on go-pprof",
	}
	skipLogsFlag = cli.BoolFlag{
		Name:  "skip-logs",
		Usage: "skip writing event|transfer logs (/logs API will be disabled)",
	}
	verifyLogsFlag = cli.BoolFlag{
		Name:   "verify-logs",
		Usage:  "verify log db at startup",
		Hidden: true,
	}
	cacheFlag = cli.Uint64Flag{
		Name:  "cache",
		Usage: "megabytes of ram allocated to trie nodes cache",
		Value: 4096,
	}
	disablePrunerFlag = cli.BoolFlag{
		Name:  "disable-pruner",
		Usage: "disable state pruner to keep all history",
	}
	enableMetricsFlag = cli.BoolFlag{
		Name:  "enable-metrics",
		Usage: "enables metrics collection",
	}
	metricsAddrFlag = cli.StringFlag{
		Name:  "metrics-addr",
		Value: "localhost:2112",
		Usage: "metrics service listening address",
	}

	enableAdminFlag = cli.BoolFlag{
		Name:  "enable-admin",
		Usage: "enables admin server",
	}
	adminAddrFlag = cli.StringFlag{
		Name:  "admin-addr",
		Value: "localhost:2113",
		Usage: "admin service listening address",
	}
	txPoolLimitPerAccountFlag = cli.Uint64Flag{
		Name:  "txpool-limit-per-account",
		Value: 16,
		Usage: "set tx limit per account in pool",
	}

	allowedTracersFlag = cli.StringFlag{
		Name:  "api-allowed-tracers",
		Value: "none",
		Usage: "comma separated list of allowed API tracers(none,all,call,prestate etc.)",
	}

	// solo mode only flags
	onDemandFlag = cli.BoolFlag{
		Name:  "on-demand",
		Usage: "create new block when there is pending transaction",
	}
	blockInterval = cli.Uint64Flag{
		Name:  "block-interval",
		Value: 10,
		Usage: "choose a custom block interval for solo mode (seconds)",
	}
	persistFlag = cli.BoolFlag{
		Name:  "persist",
		Usage: "blockchain data storage option, if set data will be saved to disk",
	}
	gasLimitFlag = cli.Uint64Flag{
		Name:  "gas-limit",
		Value: 40_000_000,
		Usage: "block gas limit(adaptive if set to 0)",
	}
	txPoolLimitFlag = cli.Uint64Flag{
		Name:  "txpool-limit",
		Value: 10000,
		Usage: "set tx limit in pool",
	}
	genesisFlag = cli.StringFlag{
		Name:  "genesis",
		Usage: "path to genesis file, if not set, the default devnet genesis will be used",
	}
)
