package main

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/log"
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
	app.Name = "Thor"
	app.Usage = "Core of VeChain"
	app.Copyright = "2017 VeChain Foundation <https://vechain.com/>"
	return app
}

func main() {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
	//glogger.Verbosity(log.Lvl(ctx.Int("verbosity")))
	//glogger.Vmodule(ctx.String("vmodule"))
	log.Root().SetHandler(glogger)

	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
