// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"github.com/urfave/cli/v3"

	"github.com/vechain/thor/v2/log"
)

func envVar(name string) cli.ValueSourceChain {
	return cli.NewValueSourceChain(cli.EnvVar("DISCO_" + name))
}

var (
	addrFlag = &cli.StringFlag{
		Name:    "addr",
		Value:   ":55555",
		Usage:   "listen address",
		Sources: envVar("ADDR"),
	}
	keyFileFlag = &cli.StringFlag{
		Name:    "keyfile",
		Usage:   "private key file path",
		Value:   defaultKeyFile(),
		Sources: envVar("KEYFILE"),
	}
	keyHexFlag = &cli.StringFlag{
		Name:    "keyhex",
		Usage:   "private key as hex",
		Sources: envVar("KEYHEX"),
	}
	natFlag = &cli.StringFlag{
		Name:    "nat",
		Value:   "none",
		Usage:   "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
		Sources: envVar("NAT"),
	}
	netRestrictFlag = &cli.StringFlag{
		Name:    "netrestrict",
		Usage:   "restrict network communication to the given IP networks (CIDR masks)",
		Sources: envVar("NETRESTRICT"),
	}
	verbosityFlag = &cli.Uint64Flag{
		Name:    "verbosity",
		Value:   log.LegacyLevelInfo,
		Usage:   "log verbosity (0-9)",
		Sources: envVar("VERBOSITY"),
	}
	enableMetricsFlag = &cli.BoolFlag{
		Name:    "enable-metrics",
		Usage:   "enables metrics collection",
		Sources: envVar("ENABLE_METRICS"),
	}
	metricsAddrFlag = &cli.StringFlag{
		Name:    "metrics-addr",
		Value:   "localhost:2112",
		Usage:   "metrics service listening address",
		Sources: envVar("METRICS_ADDR"),
	}
	disableTempDiscv5Flag = &cli.BoolFlag{
		Name:    "disable-temp-discv5",
		Hidden:  true,
		Usage:   "disable legacy discovery protocol",
		Sources: envVar("TEMP_DISCV5"),
	}
)
