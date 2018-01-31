package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/node"
	"github.com/vechain/thor/node/network"
)

func main() {
	nw := network.New()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer signal.Stop(interrupt)
		<-interrupt
		cancel()
	}()

	wg := new(sync.WaitGroup)

	go func() {
		defer wg.Done()

		node2 := node.New(node.Options{
			DataPath:    "/tmp/node_test_1",
			Bind:        ":8080",
			IP:          "1",
			Proposer:    genesis.Dev.Accounts()[1].Address,
			Beneficiary: genesis.Dev.Accounts()[1].Address,
			PrivateKey:  genesis.Dev.Accounts()[1].PrivateKey})
		node2.SetGenesisBuild(genesis.Dev.Build)

		if err := node2.Run(ctx, nw); err != nil {
			log.Println(err)
		}
	}()
	wg.Add(1)

	go func() {
		defer wg.Done()

		node1 := node.New(node.Options{
			DataPath:    "/tmp/node_test_2",
			Bind:        ":8081",
			IP:          "2",
			Proposer:    genesis.Dev.Accounts()[2].Address,
			Beneficiary: genesis.Dev.Accounts()[2].Address,
			PrivateKey:  genesis.Dev.Accounts()[2].PrivateKey})
		node1.SetGenesisBuild(genesis.Dev.Build)

		if err := node1.Run(ctx, nw); err != nil {
			log.Println(err)
		}
	}()
	wg.Add(1)

	wg.Wait()
}
