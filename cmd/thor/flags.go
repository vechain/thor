package main

import (
	"github.com/inconshreveable/log15"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	p2pPortFlag = cli.IntFlag{
		Name:  "p2pport",
		Value: 11235,
		Usage: "P2P network listening port",
	}
	apiAddrFlag = cli.StringFlag{
		Name:  "apiaddr",
		Value: "localhost:8669",
		Usage: "API service listening address",
	}
	apiCorsFlag = cli.StringFlag{
		Name:  "apicors",
		Value: "",
		Usage: "Comma separated list of domains from which to accept cross origin requests to API",
	}
	dirFlag = cli.StringFlag{
		Name:  "dir",
		Value: defaultMainDir(),
		Usage: "Main directory for configs and databases",
	}

	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Value: int(log15.LvlInfo),
		Usage: "log verbosity (0-9)",
	}
	devFlag = cli.BoolFlag{
		Name:  "dev",
		Usage: "develop mode",
	}
	beneficiaryFlag = cli.StringFlag{
		Name:  "beneficiary",
		Usage: "address for block rewards",
	}
	maxPeersFlag = cli.IntFlag{
		Name:  "maxpeers",
		Usage: "maximum number of P2P network peers (P2P network disabled if set to 0)",
		Value: 10,
	}
)
