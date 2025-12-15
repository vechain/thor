// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// disco runs a bootstrap node for the Ethereum Discovery Protocol.
package main

import (
	"crypto/ecdsa"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	cli "gopkg.in/urfave/cli.v1"

	"github.com/vechain/thor/v2/p2p/discv5/enode"
	"github.com/vechain/thor/v2/p2p/nat"
	"github.com/vechain/thor/v2/p2p/netutil"
	"github.com/vechain/thor/v2/p2p/tempdiscv5"
	"github.com/vechain/thor/v2/p2psrv"
)

var (
	version       string
	gitCommit     string
	gitTag        string
	copyrightYear string

	flags = []cli.Flag{
		addrFlag,
		keyFileFlag,
		keyHexFlag,
		natFlag,
		netRestrictFlag,
		verbosityFlag,
		enableMetricsFlag,
		metricsAddrFlag,
		disableTempDiscv5Flag,
	}
)

func run(ctx *cli.Context) error {
	lvl, err := readIntFromUInt64Flag(ctx.Uint64(verbosityFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse verbosity flag")
	}
	initLogger(lvl)

	natm, err := nat.Parse(ctx.String(natFlag.Name))
	if err != nil {
		return errors.Wrap(err, "-nat")
	}

	var key *ecdsa.PrivateKey

	if keyHex := ctx.String(keyHexFlag.Name); keyHex != "" {
		if key, err = crypto.HexToECDSA(keyHex); err != nil {
			return errors.Wrap(err, "-keyhex")
		}
	} else {
		if key, err = loadOrGenerateKeyFile(ctx.String("keyfile")); err != nil {
			return errors.Wrap(err, "-keyfile")
		}
	}

	netrestrict := ctx.String(netRestrictFlag.Name)
	var restrictList *netutil.Netlist
	if netrestrict != "" {
		restrictList, err = netutil.ParseNetlist(netrestrict)
		if err != nil {
			return errors.Wrap(err, "-netrestrict")
		}
	}

	enableTempDiscv5 := !ctx.Bool(disableTempDiscv5Flag.Name)

	opts := &p2psrv.Options{
		Name:        common.MakeName("thor", version),
		PrivateKey:  key,
		ListenAddr:  ctx.String(addrFlag.Name),
		NAT:         natm,
		DiscV5:      true,
		TempDiscV5:  enableTempDiscv5,
		NetRestrict: restrictList,
		NoDial:      true,
	}

	srv := p2psrv.New(opts, func(node *enode.Node) bool {
		// allow all nodes to be added
		return true
	})
	topic := tempdiscv5.Topic("disco")
	if err := srv.Start(nil, &topic); err != nil {
		return err
	}

	println("Disco node started")
	println("Enode URL:", srv.Self().String())

	exitSignal := handleExitSignal()
	<-exitSignal.Done()

	srv.Stop()

	return nil
}

func main() {
	versionMeta := "release"
	if gitTag == "" {
		versionMeta = "dev"
	}
	app := cli.App{
		Version:   fmt.Sprintf("%s-%s-%s", version, gitCommit, versionMeta),
		Name:      "Disco",
		Usage:     "VeChain Thor bootstrap node",
		Copyright: fmt.Sprintf("2018-%s VeChain Foundation <https://vechain.org/>", copyrightYear),
		Flags:     flags,
		Action:    run,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
