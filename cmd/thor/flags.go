// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"github.com/urfave/cli/v3"

	"github.com/vechain/thor/v2/log"
)

func envVar(name string) cli.ValueSourceChain {
	return cli.NewValueSourceChain(cli.EnvVar("THOR_" + name))
}

var (
	networkFlag = &cli.StringFlag{
		Name:    "network",
		Local:   true,
		Usage:   "the network to join (mainnet|main|testnet|test) or the path/URL to a genesis file",
		Sources: envVar("NETWORK"),
	}
	configDirFlag = &cli.StringFlag{
		Name:    "config-dir",
		Local:   true,
		Value:   defaultConfigDir(),
		Hidden:  true,
		Usage:   "directory for user global configurations",
		Sources: envVar("CONFIG_DIR"),
	}
	masterKeyStdinFlag = &cli.BoolFlag{
		Name:    "master-key-stdin",
		Local:   true,
		Usage:   "read master key from stdin",
		Hidden:  true,
		Sources: envVar("MASTER_KEY_STDIN"),
	}
	dataDirFlag = &cli.StringFlag{
		Name:    "data-dir",
		Local:   true,
		Value:   defaultDataDir(),
		Usage:   "directory for block-chain databases",
		Sources: envVar("DATA_DIR"),
	}
	beneficiaryFlag = &cli.StringFlag{
		Name:    "beneficiary",
		Local:   true,
		Usage:   "address for block rewards",
		Sources: envVar("BENEFICIARY"),
	}
	apiAddrFlag = &cli.StringFlag{
		Name:    "api-addr",
		Local:   true,
		Value:   "localhost:8669",
		Usage:   "API service listening address",
		Sources: envVar("API_ADDR"),
	}
	apiCorsFlag = &cli.StringFlag{
		Name:    "api-cors",
		Local:   true,
		Value:   "",
		Usage:   "comma separated list of domains from which to accept cross origin requests to API",
		Sources: envVar("API_CORS"),
	}
	apiTimeoutFlag = &cli.Uint64Flag{
		Name:    "api-timeout",
		Local:   true,
		Value:   10000,
		Usage:   "API request timeout value in milliseconds",
		Sources: envVar("API_TIMEOUT"),
	}
	apiCallGasLimitFlag = &cli.Uint64Flag{
		Name:    "api-call-gas-limit",
		Local:   true,
		Value:   50000000,
		Usage:   "limit contract call gas",
		Sources: envVar("API_CALL_GAS_LIMIT"),
	}
	apiBacktraceLimitFlag = &cli.Uint64Flag{
		Name:    "api-backtrace-limit",
		Local:   true,
		Value:   1000,
		Usage:   "limit the distance between 'position' and best block for subscriptions and fees APIs",
		Sources: envVar("API_BACKTRACE_LIMIT"),
	}
	apiAllowCustomTracerFlag = &cli.BoolFlag{
		Name:    "api-allow-custom-tracer",
		Local:   true,
		Usage:   "allow custom JS tracer to be used tracer API",
		Sources: envVar("API_ALLOW_CUSTOM_TRACER"),
	}
	apiLogsLimitFlag = &cli.Uint64Flag{
		Name:    "api-logs-limit",
		Local:   true,
		Value:   1000,
		Usage:   "limit the number of logs returned by /logs API",
		Sources: envVar("API_LOGS_LIMIT"),
	}
	apiEnableDeprecatedFlag = &cli.BoolFlag{
		Name:    "api-enable-deprecated",
		Local:   true,
		Usage:   "enable deprecated API endpoints (POST /accounts/{address}, POST /accounts, WS /subscriptions/beat",
		Sources: envVar("API_ENABLE_DEPRECATED"),
	}
	enableAPILogsFlag = &cli.BoolFlag{
		Name:    "enable-api-logs",
		Local:   true,
		Usage:   "enables API requests logging",
		Sources: envVar("ENABLE_API_LOGS"),
	}
	apiTxpoolFlag = &cli.BoolFlag{
		Name:    "api-enable-txpool",
		Local:   true,
		Usage:   "enable txpool REST API endpoints",
		Sources: envVar("API_ENABLE_TXPOOL"),
	}
	// db indexes flags
	logDbAdditionalIndexesFlag = &cli.BoolFlag{
		Name:    "logdb-additional-indexes",
		Local:   true,
		Usage:   "enable creation of additional indexes on startup",
		Sources: envVar("LOGDB_ADDITIONAL_INDEXES"),
	}
	// priority fees API flags
	apiPriorityFeesPercentageFlag = &cli.Uint64Flag{
		Name:    "api-priority-fees-percentage",
		Local:   true,
		Value:   5,
		Usage:   "percentage of the block base fee for priority fees calculation",
		Sources: envVar("API_PRIORITY_FEES_PERCENTAGE"),
	}
	apiSlowQueriesThresholdFlag = &cli.Uint64Flag{
		Name:    "api-slow-queries-threshold",
		Local:   true,
		Value:   0,
		Usage:   "all queries with execution time(ms) above threshold will be logged",
		Sources: envVar("API_SLOW_QUERIES_THRESHOLD"),
	}
	apiLog5xxErrorsFlag = &cli.BoolFlag{
		Name:    "api-log-5xx-errors",
		Local:   true,
		Usage:   "log all API requests resulting in 5xx status codes",
		Sources: envVar("API_LOG5XX_ERRORS"),
	}
	verbosityFlag = &cli.Uint64Flag{
		Name:    "verbosity",
		Local:   true,
		Value:   log.LegacyLevelInfo,
		Usage:   "log verbosity (0-9)",
		Sources: envVar("VERBOSITY"),
	}
	verbosityStakerFlag = &cli.Uint64Flag{
		Name:    "verbosity-staker",
		Local:   true,
		Usage:   "log verbosity for staker (0-9)",
		Value:   log.LegacyLevelError,
		Sources: envVar("VERBOSITY_STAKER"),
	}
	jsonLogsFlag = &cli.BoolFlag{
		Name:    "json-logs",
		Local:   true,
		Usage:   "output logs in JSON format",
		Sources: envVar("JSON_LOGS"),
	}
	maxPeersFlag = &cli.Uint64Flag{
		Name:    "max-peers",
		Local:   true,
		Usage:   "maximum number of P2P network peers (P2P network disabled if set to 0)",
		Value:   25,
		Sources: envVar("MAX_PEERS"),
	}
	p2pPortFlag = &cli.Uint64Flag{
		Name:    "p2p-port",
		Local:   true,
		Value:   11235,
		Usage:   "P2P network listening port",
		Sources: envVar("P2P_PORT"),
	}
	natFlag = &cli.StringFlag{
		Name:    "nat",
		Local:   true,
		Value:   "any",
		Usage:   "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
		Sources: envVar("NAT"),
	}
	bootNodeFlag = &cli.StringFlag{
		Name:    "bootnode",
		Local:   true,
		Usage:   "comma separated list of bootstrap node IDs",
		Sources: envVar("BOOTNODE"),
	}
	allowedPeersFlag = &cli.StringFlag{
		Name:    "allowed-peers",
		Local:   true,
		Hidden:  true,
		Usage:   "comma separated list of node IDs that can be connected to",
		Sources: envVar("ALLOWED_PEERS"),
	}
	targetGasLimitFlag = &cli.Uint64Flag{
		Name:    "target-gas-limit",
		Local:   true,
		Value:   0,
		Usage:   "target block gas limit (adaptive if set to 0)",
		Sources: envVar("TARGET_GAS_LIMIT"),
	}
	pprofFlag = &cli.BoolFlag{
		Name:    "pprof",
		Local:   true,
		Usage:   "turn on go-pprof",
		Sources: envVar("PPROF"),
	}
	skipLogsFlag = &cli.BoolFlag{
		Name:    "skip-logs",
		Local:   true,
		Usage:   "skip writing event|transfer logs (/logs API will be disabled)",
		Sources: envVar("SKIP_LOGS"),
	}
	verifyLogsFlag = &cli.BoolFlag{
		Name:    "verify-logs",
		Local:   true,
		Usage:   "verify log db at startup",
		Hidden:  true,
		Sources: envVar("VERIFY_LOGS"),
	}
	cacheFlag = &cli.Uint64Flag{
		Name:    "cache",
		Local:   true,
		Usage:   "megabytes of ram allocated to trie nodes cache",
		Value:   4096,
		Sources: envVar("CACHE"),
	}
	disablePrunerFlag = &cli.BoolFlag{
		Name:    "disable-pruner",
		Local:   true,
		Usage:   "disable state pruner to keep all history",
		Sources: envVar("DISABLE_PRUNER"),
	}
	enableMetricsFlag = &cli.BoolFlag{
		Name:    "enable-metrics",
		Local:   true,
		Usage:   "enables metrics collection",
		Sources: envVar("ENABLE_METRICS"),
	}
	metricsAddrFlag = &cli.StringFlag{
		Name:    "metrics-addr",
		Local:   true,
		Value:   "localhost:2112",
		Usage:   "metrics service listening address",
		Sources: envVar("METRICS_ADDR"),
	}
	enableAdminFlag = &cli.BoolFlag{
		Name:    "enable-admin",
		Local:   true,
		Usage:   "enables admin server",
		Sources: envVar("ENABLE_ADMIN"),
	}
	adminAddrFlag = &cli.StringFlag{
		Name:    "admin-addr",
		Local:   true,
		Value:   "localhost:2113",
		Usage:   "admin service listening address",
		Sources: envVar("ADMIN_ADDR"),
	}
	txPoolLimitPerAccountFlag = &cli.Uint64Flag{
		Name:    "txpool-limit-per-account",
		Local:   true,
		Value:   128,
		Usage:   "set tx limit per account in pool",
		Sources: envVar("TX_POOL_LIMIT_PER_ACCOUNT"),
	}
	allowedTracersFlag = &cli.StringFlag{
		Name:    "api-allowed-tracers",
		Local:   true,
		Value:   "none",
		Usage:   "comma separated list of allowed API tracers(none,all,call,prestate etc.)",
		Sources: envVar("API_ALLOWED_TRACERS"),
	}
	minEffectivePriorityFeeFlag = &cli.Uint64Flag{
		Name:    "min-effective-priority-fee",
		Local:   true,
		Value:   0,
		Usage:   "set a minimum effective priority fee for transactions to be included in the block proposed by the block proposer",
		Sources: envVar("MIN_EFFECTIVE_PRIORITY_FEE"),
	}

	// master key only flags
	importMasterKeyFlag = &cli.BoolFlag{
		Name:  "import",
		Local: true,
		Usage: "import master key from keystore",
	}
	exportMasterKeyFlag = &cli.BoolFlag{
		Name:  "export",
		Local: true,
		Usage: "export master key to keystore",
	}

	// solo mode only flags
	onDemandFlag = &cli.BoolFlag{
		Name:    "on-demand",
		Local:   true,
		Usage:   "create new block when there is pending transaction, may result in block produced in the future timestamp",
		Sources: envVar("ON_DEMAND"),
	}
	blockInterval = &cli.Uint64Flag{
		Name:    "block-interval",
		Local:   true,
		Value:   10,
		Usage:   "choose a custom block interval for solo mode (seconds)",
		Sources: envVar("BLOCK_INTERVAL"),
	}
	persistFlag = &cli.BoolFlag{
		Name:    "persist",
		Local:   true,
		Usage:   "blockchain data storage option, if set data will be saved to disk",
		Sources: envVar("PERSIST"),
	}
	gasLimitFlag = &cli.Uint64Flag{
		Name:    "gas-limit",
		Local:   true,
		Value:   40_000_000,
		Usage:   "block gas limit(adaptive if set to 0)",
		Sources: envVar("GAS_LIMIT"),
	}
	txPoolLimitFlag = &cli.Uint64Flag{
		Name:    "txpool-limit",
		Local:   true,
		Value:   10000,
		Usage:   "set tx limit in pool",
		Sources: envVar("TX_POOL_LIMIT"),
	}
	genesisFlag = &cli.StringFlag{
		Name:    "genesis",
		Local:   true,
		Usage:   "path or URL to genesis file, if not set, the default devnet genesis will be used",
		Sources: envVar("GENESIS"),
	}
)
