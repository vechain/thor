// disco runs a bootstrap node for the Ethereum Discovery Protocol.
package main

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/p2p/netutil"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	version   string
	gitCommit string
	release   = "dev"
)

func newApp() *cli.App {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Disco"
	app.Usage = "VeChain Thor bootstrap node"
	app.Copyright = "2018 VeChain Foundation <https://vechain.org/>"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "addr",
			Value: ":55555",
			Usage: "listen address",
		},
		cli.StringFlag{
			Name:  "key",
			Usage: "private key file path (defaults to ~/.thor-disco.key if omitted)",
		},
		cli.StringFlag{
			Name:  "keyhex",
			Usage: "private key as hex (for testing)",
		},
		cli.StringFlag{
			Name:  "nat",
			Value: "none",
			Usage: "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
		},
		cli.StringFlag{
			Name:  "netrestrict",
			Usage: "restrict network communication to the given IP networks (CIDR masks)",
		},
		cli.IntFlag{
			Name:  "verbosity",
			Value: int(log.LvlInfo),
			Usage: "log verbosity (0-9)",
		},
		cli.StringFlag{
			Name:  "vmodule",
			Usage: "log verbosity pattern",
		},
	}
	app.Action = func(ctx *cli.Context) (err error) {
		glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
		glogger.Verbosity(log.Lvl(ctx.Int("verbosity")))
		glogger.Vmodule(ctx.String("vmodule"))
		log.Root().SetHandler(glogger)

		natm, err := nat.Parse(ctx.String("nat"))
		if err != nil {
			return err
		}

		key, err := loadKey(ctx)
		if err != nil {
			return err
		}

		netrestrict := ctx.String("netrestrict")
		var restrictList *netutil.Netlist
		if netrestrict != "" {
			restrictList, err = netutil.ParseNetlist(netrestrict)
			if err != nil {
				return err
			}
		}

		net, err := discv5.ListenUDP(key, ctx.String("addr"), natm, "", restrictList)
		if err != nil {
			return err
		}
		fmt.Println(net.Self().String())

		select {}
	}
	return app
}

func loadKey(ctx *cli.Context) (key *ecdsa.PrivateKey, err error) {
	// use hex key if there
	hexKey := ctx.String("keyhex")
	if hexKey != "" {
		return crypto.HexToECDSA(hexKey)
	}

	keyFile := ctx.String("key")
	if keyFile == "" {
		// no file specified, use default file path
		home, err := homeDir()
		if err != nil {
			return nil, err
		}
		keyFile = filepath.Join(home, ".thor-disco.key")
	} else if !filepath.IsAbs(keyFile) {
		// resolve to absolute path
		keyFile, err = filepath.Abs(keyFile)
		if err != nil {
			return nil, err
		}
	}

	// try to load from file
	if key, err = crypto.LoadECDSA(keyFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		return key, nil
	}

	// no such file, generate new key and write in
	key, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	if err := crypto.SaveECDSA(keyFile, key); err != nil {
		return nil, err
	}
	return key, nil
}

func homeDir() (string, error) {
	// try to get HOME env
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	user, err := user.Current()
	if err != nil {
		return "", err
	}
	if user.HomeDir != "" {
		return user.HomeDir, nil
	}

	return os.Getwd()
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
