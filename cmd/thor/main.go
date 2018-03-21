package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/p2psrv"
	cli "gopkg.in/urfave/cli.v1"
)

var boot1 = "enode://ec0ccfaeefa53c6a7ec73ca36940c911902d1f6f9da7567d05c44d1aa841b309260f7b228008331b61c8890ece0297eb0c3541af1a51fd5fcc749bee9104e64a@192.168.31.182:55555"
var boot2 = "enode://0cc5f5ffb5d9098c8b8c62325f3797f56509bff942704687b6530992ac706e2cb946b90a34f1f19548cd3c7baccbcaea354531e5983c7d1bc0dee16ce4b6440b@40.118.3.223:30305"

var key = func() string {
	k, _ := crypto.GenerateKey()
	return hex.EncodeToString(crypto.FromECDSA(k))
}()

func mustHexToECDSA(k string) *ecdsa.PrivateKey {
	pk, err := crypto.HexToECDSA(k)
	if err != nil {
		panic(err)
	}
	return pk
}

var (
	version   string
	gitCommit string
	release   = "dev"
)

func newApp() *cli.App {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Thor"
	app.Usage = "Core of VeChain"
	app.Copyright = "2017 VeChain Foundation <https://vechain.com/>"
	return app
}

func main() {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
	glogger.Verbosity(log.LvlInfo)
	//glogger.Vmodule(ctx.String("vmodule"))
	log.Root().SetHandler(glogger)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	op := Options{
		DataPath:    "/tmp/node_test_1",
		Bind:        ":8081",
		Proposer:    genesis.Dev.Accounts()[1].Address,
		Beneficiary: genesis.Dev.Accounts()[1].Address,
		PrivateKey:  genesis.Dev.Accounts()[1].PrivateKey}

	srv := p2psrv.New(
		&p2psrv.Options{
			PrivateKey:     mustHexToECDSA(key),
			MaxPeers:       25,
			ListenAddr:     ":40001",
			BootstrapNodes: []*discover.Node{discover.MustParseNode(boot1), discover.MustParseNode(boot2)},
		})

	stop, err := Start(op, srv)
	if err != nil {
		panic(err)
	}

	<-interrupt
	stop()

	// glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
	// //glogger.Verbosity(log.Lvl(ctx.Int("verbosity")))
	// //glogger.Vmodule(ctx.String("vmodule"))
	// log.Root().SetHandler(glogger)

	// if err := newApp().Run(os.Args); err != nil {
	// 	fmt.Fprintln(os.Stderr, err)
	// 	os.Exit(1)
	// }
}
