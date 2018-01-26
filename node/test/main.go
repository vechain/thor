package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/vechain/thor/node"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	node := node.New(node.Options{
		DataPath: "/Users/hanxiao/Desktop/asfasfd",
		Bind:     ":8080"})

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer signal.Stop(interrupt)
		<-interrupt
		cancel()
	}()

	if err := node.Run(ctx); err != nil {
		fmt.Println(err)
	}
}
