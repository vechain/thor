// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"gopkg.in/urfave/cli.v1"

	"github.com/vechain/thor/v2/log"
)

var (
	addrFlag = cli.StringFlag{
		Name:  "addr",
		Value: ":55555",
		Usage: "listen address",
	}
	keyFileFlag = cli.StringFlag{
		Name:  "keyfile",
		Usage: "private key file path",
		Value: defaultKeyFile(),
	}
	keyHexFlag = cli.StringFlag{
		Name:  "keyhex",
		Usage: "private key as hex",
	}
	natFlag = cli.StringFlag{
		Name:  "nat",
		Value: "none",
		Usage: "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
	}
	netRestrictFlag = cli.StringFlag{
		Name:  "netrestrict",
		Usage: "restrict network communication to the given IP networks (CIDR masks)",
	}
	verbosityFlag = cli.UintFlag{
		Name:  "verbosity",
		Value: log.LegacyLevelWarn,
		Usage: "log verbosity (0-9)",
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
	disableTempDiscv5Flag = cli.BoolFlag{
		Name:   "disable-temp-discv5",
		Hidden: true,
		Usage:  "disable legacy discovery protocol",
	}
)
