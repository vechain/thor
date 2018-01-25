package node

import (
	"context"
	"os"
	"os/signal"
	"testing"
)

func Test_Start(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	node := New(Options{
		dataPath: "/Users/hanxiao/Desktop/asfasfd",
		bind:     ""})

	ctx, _ := context.WithCancel(context.Background())

	// go func() {
	// 	defer signal.Stop(interrupt)
	// 	<-interrupt
	// 	cancel()
	// }()

	t.Log(node.Run(ctx))
}
