package main

import (
	"github.com/inconshreveable/log15"
	cli "gopkg.in/urfave/cli.v1"
)

var appFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "p2paddr",
		Value: ":11235",
		Usage: "p2p listen addr",
	},
	cli.StringFlag{
		Name:  "apiaddr",
		Value: "127.0.0.1:8669",
		Usage: "api server addr",
	},
	cli.StringFlag{
		Name:  "datadir",
		Value: "/tmp/thor-data",
		Usage: "chain data path",
	},
	cli.IntFlag{
		Name:  "verbosity",
		Value: int(log15.LvlInfo),
		Usage: "log verbosity (0-9)",
	},
	cli.BoolFlag{
		Name:  "devnet",
		Usage: "develop network",
	},
	cli.StringFlag{
		Name:  "beneficiary",
		Usage: "address of beneficiary",
	},
	cli.IntFlag{
		Name:  "maxpeers",
		Usage: "maximum number of network peers (network disabled if set to 0)",
		Value: 10,
	},
}
