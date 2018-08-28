package subscriptions

import (
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type Subscriptions struct {
	chain *chain.Chain
	done  chan struct{}
	wg    sync.WaitGroup
}

func New(chain *chain.Chain) *Subscriptions {
	return &Subscriptions{chain: chain, done: make(chan struct{})}
}

func (s *Subscriptions) handleBlockReader(w http.ResponseWriter, req *http.Request) (*BlockReader, error) {
	position, err := s.parseBlockID(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "pos"))
	}
	return NewBlockReader(s.chain, position), nil
}

func (s *Subscriptions) handleEventReader(w http.ResponseWriter, req *http.Request) (*EventReader, error) {
	position, err := s.parseBlockID(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "pos"))
	}
	address, err := parseAddress(req.URL.Query().Get("addr"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "addr"))
	}
	t0, err := parseTopic(req.URL.Query().Get("t0"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "t0"))
	}
	t1, err := parseTopic(req.URL.Query().Get("t1"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "t1"))
	}
	t2, err := parseTopic(req.URL.Query().Get("t2"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "t2"))
	}
	t3, err := parseTopic(req.URL.Query().Get("t3"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "t3"))
	}
	t4, err := parseTopic(req.URL.Query().Get("t4"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "t4"))
	}
	eventFilter := &EventFilter{
		Address: address,
		Topic0:  t0,
		Topic1:  t1,
		Topic2:  t2,
		Topic3:  t3,
		Topic4:  t4,
	}
	return NewEventReader(s.chain, position, eventFilter), nil
}

func (s *Subscriptions) handleTransferReader(w http.ResponseWriter, req *http.Request) (*TransferReader, error) {
	position, err := s.parseBlockID(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "pos"))
	}
	txOrigin, err := parseAddress(req.URL.Query().Get("txOrigin"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "txOrigin"))
	}
	sender, err := parseAddress(req.URL.Query().Get("sender"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "sender"))
	}
	recipient, err := parseAddress(req.URL.Query().Get("recipient"))
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "recipient"))
	}
	transferFilter := &TransferFilter{
		TxOrigin:  txOrigin,
		Sender:    sender,
		Recipient: recipient,
	}
	return NewTransferReader(s.chain, position, transferFilter), nil
}

type Read func() ([]interface{}, bool, error)

var upgrader = websocket.Upgrader{}

func (s *Subscriptions) handleSubject(w http.ResponseWriter, req *http.Request) error {
	s.wg.Add(1)
	defer s.wg.Done()

	var read Read
	switch mux.Vars(req)["subject"] {
	case "block":
		blockReader, err := s.handleBlockReader(w, req)
		if err != nil {
			return err
		}
		read = blockReader.Read
	case "event":
		eventReader, err := s.handleEventReader(w, req)
		if err != nil {
			return err
		}
		read = eventReader.Read
	case "transfer":
		transferReader, err := s.handleTransferReader(w, req)
		if err != nil {
			return err
		}
		read = transferReader.Read
	default:
		return utils.HTTPError(errors.New("not found"), http.StatusNotFound)
	}

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := s.pipe(conn, read); err != nil {
		return conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
	}
	return conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""))
}
func (s *Subscriptions) pipe(conn *websocket.Conn, read Read) error {
	ticker := s.chain.NewTicker()
	for {
		select {
		case <-s.done:
			return nil
		case <-ticker.C():
			for {
				msgs, hasMore, err := read()
				if err != nil {
					return err
				}
				for _, msg := range msgs {
					if err := conn.WriteJSON(msg); err != nil {
						return err
					}
				}
				if !hasMore {
					break
				}
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

func (s *Subscriptions) Close() {
	close(s.done)
	s.wg.Wait()
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/{subject}").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(s.handleSubject))
}
