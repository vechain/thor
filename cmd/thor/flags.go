// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"github.com/inconshreveable/log15"
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
	apiTimeoutFlag = cli.IntFlag{
		Name:  "api-timeout",
		Value: 10000,
		Usage: "API request timeout value in milliseconds",
	}
	apiCallGasLimitFlag = cli.IntFlag{
		Name:  "api-call-gas-limit",
		Value: 50000000,
		Usage: "limit contract call gas",
	}
	apiBacktraceLimitFlag = cli.IntFlag{
		Name:  "api-backtrace-limit",
		Value: 1000,
		Usage: "limit the distance between 'position' and best block for subscriptions APIs",
	}
	apiAllowCustomTracerFlag = cli.BoolFlag{
		Name:  "api-allow-custom-tracer",
		Usage: "allow custom JS tracer to be used tracer API",
	}
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Value: int(log15.LvlInfo),
		Usage: "log verbosity (0-9)",
	}

	maxPeersFlag = cli.IntFlag{
		Name:  "max-peers",
		Usage: "maximum number of P2P network peers (P2P network disabled if set to 0)",
		Value: 25,
	}
	p2pPortFlag = cli.IntFlag{
		Name:  "p2p-port",
		Value: 11235,
		Usage: "P2P network listening port",
	}
	natFlag = cli.StringFlag{
		Name:  "nat",
		Value: "any",
		Usage: "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
	}
	onDemandFlag = cli.BoolFlag{
		Name:  "on-demand",
		Usage: "create new block when there is pending transaction",
	}
	persistFlag = cli.BoolFlag{
		Name:  "persist",
		Usage: "blockchain data storage option, if set data will be saved to disk",
	}
	gasLimitFlag = cli.IntFlag{
		Name:  "gas-limit",
		Value: 10000000,
		Usage: "block gas limit(adaptive if set to 0)",
	}
	importMasterKeyFlag = cli.BoolFlag{
		Name:  "import",
		Usage: "import master key from keystore",
	}
	exportMasterKeyFlag = cli.BoolFlag{
		Name:  "export",
		Usage: "export master key to keystore",
	}
	targetGasLimitFlag = cli.IntFlag{
		Name:  "target-gas-limit",
		Value: 0,
		Usage: "target block gas limit (adaptive if set to 0)",
	}
	bootNodeFlag = cli.StringFlag{
		Name:  "bootnode",
		Usage: "comma separated list of bootnode IDs",
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
	cacheFlag = cli.IntFlag{
		Name:  "cache",
		Usage: "megabytes of ram allocated to trie nodes cache",
		Value: 4096,
	}
	disablePrunerFlag = cli.BoolFlag{
		Name:  "disable-pruner",
		Usage: "disable state pruner to keep all history",
	}
	txPoolLimitFlag = cli.IntFlag{
		Name:  "txpool-limit",
		Value: 10000,
		Usage: "set tx limit in pool",
	}
	txPoolLimitPerAccountFlag = cli.IntFlag{
		Name:  "txpool-limit-per-account",
		Value: 16,
		Usage: "set tx limit per account in pool",
	}
)
