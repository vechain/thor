// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// disco runs a bootstrap node for the Ethereum Discovery Protocol.
package main

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/p2p/netutil"
	"github.com/pkg/errors"
	cli "gopkg.in/urfave/cli.v1"

	"github.com/vechain/thor/v2/cmd/thor/httpserver"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/metrics"
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
	}
)

func run(ctx *cli.Context) error {
	lvl, err := readIntFromUInt64Flag(ctx.Uint64(verbosityFlag.Name))
	if err != nil {
		return errors.Wrap(err, "parse verbosity flag")
	}
	initLogger(lvl)

	natm, err := nat.Parse(ctx.String("nat"))
	if err != nil {
		return errors.Wrap(err, "-nat")
	}

	var key *ecdsa.PrivateKey

	if keyHex := ctx.String("keyhex"); keyHex != "" {
		if key, err = crypto.HexToECDSA(keyHex); err != nil {
			return errors.Wrap(err, "-keyhex")
		}
	} else {
		if key, err = loadOrGenerateKeyFile(ctx.String("keyfile")); err != nil {
			return errors.Wrap(err, "-keyfile")
		}
	}

	netrestrict := ctx.String("netrestrict")
	var restrictList *netutil.Netlist
	if netrestrict != "" {
		restrictList, err = netutil.ParseNetlist(netrestrict)
		if err != nil {
			return errors.Wrap(err, "-netrestrict")
		}
	}

	addr, err := net.ResolveUDPAddr("udp", ctx.String("addr"))
	if err != nil {
		return errors.Wrap(err, "-addr")
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	realAddr := conn.LocalAddr().(*net.UDPAddr)
	if natm != nil {
		if !realAddr.IP.IsLoopback() {
			go nat.Map(natm, nil, "udp", realAddr.Port, realAddr.Port, "ethereum discovery")
		}
		// TODO: react to external IP changes over time.
		if ext, err := natm.ExternalIP(); err == nil {
			realAddr = &net.UDPAddr{IP: ext, Port: realAddr.Port}
		}
	}
	net, err := discv5.ListenUDP(key, conn, realAddr, "", restrictList)
	if err != nil {
		return err
	}
	defer net.Close()
	fmt.Println("Running", net.Self().String())

	exitSignal := handleExitSignal()

	if ctx.Bool(enableMetricsFlag.Name) {
		metrics.InitializePrometheusMetrics()
		url, closeFunc, err := httpserver.StartMetricsServer(ctx.String(metricsAddrFlag.Name))
		if err != nil {
			return fmt.Errorf("unable to start metrics server - %w", err)
		}
		log.Info("metrics server started", "url", url)
		defer closeFunc()
		pollMetrics(exitSignal, net)
	}

	<-exitSignal.Done()

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
