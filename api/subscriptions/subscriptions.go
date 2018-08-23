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
	chain *chain.Chain
}

func New(chain *chain.Chain) *Subscriptions {
	return &Subscriptions{chain}
}

func (s *Subscriptions) handleSubscribeBlock(w http.ResponseWriter, req *http.Request) error {
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	bid, err := s.parseBlockID(req.URL.Query().Get("bid"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "bid"))
	}
	blockSub := NewBlockSub(s.chain, bid)
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
	bid, err := s.parseBlockID(req.URL.Query().Get("bid"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "bid"))
	}
	address, err := parseAddress(req.URL.Query().Get("addr"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "addr"))
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
	eventSub := NewEventSub(s.chain, eventFilter)
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

func (s *Subscriptions) parseBlockID(bid string) (thor.Bytes32, error) {
	if bid == "" {
		return s.chain.BestBlock().Header().ID(), nil
	}
	return thor.ParseBytes32(bid)
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

func parseAddress(addr string) (*thor.Address, error) {
	if addr == "" {
		return nil, nil
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return nil, err
	}
	return &address, nil
}

func (s *Subscriptions) handleSubscribeTransfer(w http.ResponseWriter, req *http.Request) error {
	var upgrader = websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	bid, err := s.parseBlockID(req.URL.Query().Get("bid"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "bid"))
	}
	txOrigin, err := parseAddress(req.URL.Query().Get("txOrigin"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "txOrigin"))
	}
	sender, err := parseAddress(req.URL.Query().Get("sender"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "sender"))
	}
	recipient, err := parseAddress(req.URL.Query().Get("recipient"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "recipient"))
	}
	transferFilter := &TransferFilter{
		FromBlock: bid,
		TxOrigin:  txOrigin,
		Sender:    sender,
		Recipient: recipient,
	}
	transferSub := NewTransferSub(s.chain, transferFilter)
	for {
		remains, removes, err := transferSub.Read(req.Context())
		if err != nil {
			return err
		}
		for _, removed := range removes {
			if err := conn.WriteJSON(convertTransfer(removed, true)); err != nil {
				return err
			}
		}
		for _, remained := range remains {
			if err := conn.WriteJSON(convertTransfer(remained, false)); err != nil {
				return err
			}
		}
	}
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/block").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeBlock))
	sub.Path("/event").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeEvent))
	sub.Path("/transfer").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubscribeTransfer))

}
