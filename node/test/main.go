package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/node"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	node := node.New(node.Options{
		DataPath:    "/tmp/node_test",
		Bind:        ":8080",
		Proposer:    genesis.Dev.Accounts()[0].Address,
		Beneficiary: genesis.Dev.Accounts()[0].Address,
		PrivateKey:  genesis.Dev.Accounts()[0].PrivateKey})
	node.SetGenesisBuild(genesis.Dev.Build)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer signal.Stop(interrupt)
		<-interrupt
		cancel()
	}()

	if err := node.Run(ctx); err != nil {
		log.Println(err)
	}
}
