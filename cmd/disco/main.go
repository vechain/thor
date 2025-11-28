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
	cli "gopkg.in/urfave/cli.v1"

	"github.com/vechain/thor/v2/cmd/thor/httpserver"
	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/p2p/discv5"
	"github.com/vechain/thor/v2/p2p/nat"
	"github.com/vechain/thor/v2/p2p/netutil"
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
	}
)

func run(ctx *cli.Context) error {
	lvl, err := readIntFromUInt64Flag(ctx.Uint64(verbosityFlag.Name))
	if err != nil {
		return fmt.Errorf("parse verbosity flag: %w", err)
	}
	initLogger(lvl)

	natm, err := nat.Parse(ctx.String("nat"))
	if err != nil {
		return fmt.Errorf("-nat: %w", err)
	}

	var key *ecdsa.PrivateKey

	if keyHex := ctx.String("keyhex"); keyHex != "" {
		if key, err = crypto.HexToECDSA(keyHex); err != nil {
			return fmt.Errorf("-keyhex: %w", err)
		}
	} else {
		if key, err = loadOrGenerateKeyFile(ctx.String("keyfile")); err != nil {
			return fmt.Errorf("-keyfile: %w", err)
		}
	}

	netrestrict := ctx.String("netrestrict")
	var restrictList *netutil.Netlist
	if netrestrict != "" {
		restrictList, err = netutil.ParseNetlist(netrestrict)
		if err != nil {
			return fmt.Errorf("-netrestrict: %w", err)
		}
	}

	addr, err := net.ResolveUDPAddr("udp", ctx.String("addr"))
	if err != nil {
		return fmt.Errorf("-addr: %w", err)
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
		fmt.Println("metrics server listening", url)
		defer closeFunc()
		go pollMetrics(exitSignal, net)
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
