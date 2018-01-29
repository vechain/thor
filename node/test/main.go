package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/vechain/thor/fortest"

	"github.com/vechain/thor/node"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	node := node.New(node.Options{
		DataPath:    "/tmp/node_test",
		Bind:        ":8080",
		Proposer:    fortest.Accounts[0].Address,
		Beneficiary: fortest.Accounts[0].Address,
		PrivateKey:  fortest.Accounts[0].PrivateKey})
	node.SetGenesisBuild(fortest.BuildGenesis)

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
