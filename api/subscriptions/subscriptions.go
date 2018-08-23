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

func (s *Subscriptions) handleSubscribeBlock(w http.ResponseWriter, req *http.Request) error {
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	var bid thor.Bytes32
	if req.URL.Query().Get("bid") == "" {
		bid = s.chain.BestBlock().Header().ID()
	} else {
		fbid, err := thor.ParseBytes32(req.URL.Query().Get("bid"))
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "bid"))
		}
		bid = fbid
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

func (s *Subscriptions) handleSubscribeEvent(w http.ResponseWriter, req *http.Request) error {
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	var bid thor.Bytes32
	if req.URL.Query().Get("bid") == "" {
		bid = s.chain.BestBlock().Header().ID()
	} else {
		fbid, err := thor.ParseBytes32(req.URL.Query().Get("bid"))
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "bid"))
		}
		bid = fbid
	}
	var address *thor.Address
	if req.URL.Query().Get("addr") != "" {
		addr, err := thor.ParseAddress(req.URL.Query().Get("addr"))
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "addr"))
		}
		address = &addr
	}
	t0, err := parseTopic(req.URL.Query().Get("t0"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "t0"))
	}
	t1, err := parseTopic(req.URL.Query().Get("t1"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "t1"))
	}
	t2, err := parseTopic(req.URL.Query().Get("t2"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "t2"))
	}
	t3, err := parseTopic(req.URL.Query().Get("t3"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "t3"))
	}
	t4, err := parseTopic(req.URL.Query().Get("t4"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "t4"))
	}
	eventFilter := &EventFilter{
		FromBlock: bid,
		Address:   address,
		Topic0:    t0,
		Topic1:    t1,
		Topic2:    t2,
		Topic3:    t3,
		Topic4:    t4,
	}
	eventSub := NewEventSub(s.ch, s.chain, eventFilter)
	for {
		remains, removes, err := eventSub.Read(req.Context())
		if err != nil {
			return err
		}
		for _, removed := range removes {
			if err := conn.WriteJSON(convertEvent(removed, true)); err != nil {
				return err
			}
		}
		for _, remained := range remains {
			if err := conn.WriteJSON(convertEvent(remained, false)); err != nil {
				return err
			}
		}
	}
}

func parseTopic(t string) (*thor.Bytes32, error) {
	if t == "" {
		return nil, nil
	}
	topic, err := thor.ParseBytes32(t)
	if err != nil {
		return nil, err
	}
	return &topic, nil
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/block").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeBlock))
	sub.Path("/event").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeEvent))

}
