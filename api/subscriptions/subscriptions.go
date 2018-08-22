package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"

	"github.com/pkg/errors"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/thor"
)

type Subscriptions struct {
	ch    chan struct{}
	chain *chain.Chain
}

func New(ch chan struct{}, chain *chain.Chain) *Subscriptions {
	return &Subscriptions{ch, chain}
}

func (s *Subscriptions) handleSubscribeBlocks(w http.ResponseWriter, req *http.Request) error {
	ctx, cancel := context.WithTimeout(req.Context(), time.Second*30)
	defer cancel()
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		fmt.Println("upgrade:", err)
		return err
	}
	defer conn.Close()
	bid, err := thor.ParseBytes32(req.URL.Query().Get("bid"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "bid"))
	}
	blockSub := NewBlockSub(s.ch, s.chain, bid)
	remained := make(chan []*block.Block, 1)
	removed := make(chan []*block.Block, 1)
	readErr := make(chan error, 1)
	go func() {
		remains, removes, err := blockSub.Read(ctx)
		remained <- remains
		removed <- removes
		readErr <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-readErr:
		if err != nil {
			return err
		}
		// TODO
		b, err := json.Marshal(<-remained)
		if err != nil {
			return err
		}
		return conn.WriteMessage(websocket.BinaryMessage, b)
	}
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/block").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeBlocks))
}
