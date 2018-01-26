package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/vechain/thor/api"
	"github.com/vechain/thor/chain"
)

func restfulService(ctx context.Context, listener net.Listener, chain *chain.Chain, stateC stateCreater) {
	srv := http.Server{
		Handler: api.NewHTTPHandler(chain, stateC)}

	go func() {
		defer srv.Shutdown(context.TODO())
		<-ctx.Done()
	}()

	if err := srv.Serve(listener); err != http.ErrServerClosed {
		log.Fatalln(err)
	}

	fmt.Println("restfulService exit")
}
