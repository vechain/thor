package main

import (
	"github.com/inconshreveable/log15"
	cli "gopkg.in/urfave/cli.v1"
)

var appFlags = []cli.Flag{
	cli.IntFlag{
		Name:  "p2pport",
		Value: 11235,
		Usage: "P2P network listening port",
	},
	cli.StringFlag{
		Name:  "apiaddr",
		Value: "localhost:8669",
		Usage: "API service listening address",
	},
	cli.StringFlag{
		Name:  "apicors",
		Value: "",
		Usage: "Comma separated list of domains from which to accept cross origin requests to API",
	},
	cli.StringFlag{
		Name:  "dir",
		Value: defaultMainDir(),
		Usage: "Main directory for configs and databases",
	},
	cli.IntFlag{
		Name:  "verbosity",
		Value: int(log15.LvlInfo),
		Usage: "log verbosity (0-9)",
	},
	cli.BoolFlag{
		Name:  "devnet,dev",
		Usage: "develop network",
	},
	cli.StringFlag{
		Name:  "beneficiary",
		Usage: "address of beneficiary",
	},
	cli.IntFlag{
		Name:  "maxpeers",
		Usage: "maximum number of P2P network peers (P2P network disabled if set to 0)",
		Value: 10,
	},
}
