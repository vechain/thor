package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/node"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	pk, err := crypto.HexToECDSA("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")
	if err != nil {
		log.Fatalln(err)
	}
	privateKey, err := crypto.ToECDSA(crypto.FromECDSA(pk))
	if err != nil {
		log.Fatalln(err)
	}

	node := node.New(node.Options{
		DataPath:   "/Users/hanxiao/Desktop/asfasfd",
		Bind:       ":8080",
		PrivateKey: privateKey})

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
