package subscriptions

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
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
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	bid, err := thor.ParseBytes32(req.URL.Query().Get("bid"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "bid"))
	}
	blockSub := NewBlockSub(s.ch, s.chain, bid)
	for {
		remains, removes, err := blockSub.Read(req.Context())
		if err != nil {
			return err
		}
		for _, removed := range removes {
			blk, err := convertBlock(removed, true)
			if err != nil {
				return err
			}
			if err := conn.WriteJSON(blk); err != nil {
				return err
			}
		}
		for _, remained := range remains {
			blk, err := convertBlock(remained, false)
			if err != nil {
				return err
			}
			if err := conn.WriteJSON(blk); err != nil {
				return err
			}
		}
	}
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/block").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeBlocks))
}
