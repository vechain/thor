package main

import (
	"github.com/vechain/thor/v2/log"
	"gopkg.in/urfave/cli.v1"
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
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Value: log.LegacyLevelWarn,
		Usage: "log verbosity (0-9)",
	}
)
