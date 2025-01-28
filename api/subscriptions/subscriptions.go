// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

const txQueueSize = 20

type Subscriptions struct {
	backtraceLimit    uint32
	enabledDeprecated bool
	repo              *chain.Repository
	upgrader          *websocket.Upgrader
	pendingTx         *pendingTx
	done              chan struct{}
	wg                sync.WaitGroup
	beat2Cache        *messageCache[Beat2Message]
	beatCache         *messageCache[BeatMessage]
}

type msgReader interface {
	Read() (msgs []interface{}, hasMore bool, err error)
}

var (
	logger = log.WithContext("pkg", "subscriptions")
)

const (
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 7) / 10
)

func New(repo *chain.Repository, allowedOrigins []string, backtraceLimit uint32, txpool *txpool.TxPool, enabledDeprecated bool) *Subscriptions {
	sub := &Subscriptions{
		backtraceLimit:    backtraceLimit,
		repo:              repo,
		enabledDeprecated: enabledDeprecated,
		upgrader: &websocket.Upgrader{
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				for _, allowedOrigin := range allowedOrigins {
					if allowedOrigin == origin || allowedOrigin == "*" {
						return true
					}
				}
				return false
			},
		},
		pendingTx:  newPendingTx(txpool),
		done:       make(chan struct{}),
		beat2Cache: newMessageCache[Beat2Message](backtraceLimit),
		beatCache:  newMessageCache[BeatMessage](backtraceLimit),
	}

	sub.wg.Add(1)
	go func() {
		defer sub.wg.Done()

		sub.pendingTx.DispatchLoop(sub.done)
	}()
	return sub
}

func (s *Subscriptions) handleBlockReader(_ http.ResponseWriter, req *http.Request) (msgReader, error) {
	position, err := s.parsePosition(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, err
	}
	return newBlockReader(s.repo, position), nil
}

func (s *Subscriptions) handleEventReader(w http.ResponseWriter, req *http.Request) (msgReader, error) {
	position, err := s.parsePosition(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, err
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
	return newEventReader(s.repo, position, eventFilter), nil
}

func (s *Subscriptions) handleTransferReader(_ http.ResponseWriter, req *http.Request) (msgReader, error) {
	position, err := s.parsePosition(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, err
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
	return newTransferReader(s.repo, position, transferFilter), nil
}

func (s *Subscriptions) handleBeatReader(w http.ResponseWriter, req *http.Request) (msgReader, error) {
	position, err := s.parsePosition(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, err
	}
	return newBeatReader(s.repo, position, s.beatCache), nil
}

func (s *Subscriptions) handleBeat2Reader(_ http.ResponseWriter, req *http.Request) (msgReader, error) {
	position, err := s.parsePosition(req.URL.Query().Get("pos"))
	if err != nil {
		return nil, err
	}
	return newBeat2Reader(s.repo, position, s.beat2Cache), nil
}

func (s *Subscriptions) handlePendingTransactions(w http.ResponseWriter, req *http.Request) error {
	s.wg.Add(1)
	defer s.wg.Done()

	conn, closed, err := s.setupConn(w, req)
	// since the conn is hijacked here, no error should be returned in lines below
	if err != nil {
		logger.Debug("upgrade to websocket", "err", err)
		return nil
	}
	defer s.closeConn(conn, err)

	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()

	txCh := make(chan *tx.Transaction, txQueueSize)
	s.pendingTx.Subscribe(txCh)
	defer func() {
		s.pendingTx.Unsubscribe(txCh)
		close(txCh)
	}()

	for {
		select {
		case tx := <-txCh:
			err = conn.WriteJSON(&PendingTxIDMessage{ID: tx.ID()})
			if err != nil {
				return nil
			}
		case <-s.done:
			return nil
		case <-closed:
			return nil
		case <-pingTicker.C:
			conn.WriteMessage(websocket.PingMessage, nil)
		}
	}
}

func (s *Subscriptions) setupConn(w http.ResponseWriter, req *http.Request) (*websocket.Conn, chan struct{}, error) {
	conn, err := s.upgrader.Upgrade(w, req, nil)
	if err != nil {
		return nil, nil, err
	}
	conn.SetReadLimit(100 * 1024) // 100 KB

	closed := make(chan struct{})
	// start read loop to handle close event
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				logger.Debug("websocket read err", "err", err)
				close(closed)
				break
			}
		}
	}()

	return conn, closed, nil
}

func (s *Subscriptions) closeConn(conn *websocket.Conn, err error) {
	var closeMsg []byte
	if err != nil {
		closeMsg = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error())
	} else {
		closeMsg = websocket.FormatCloseMessage(websocket.CloseGoingAway, "")
	}

	if err := conn.WriteMessage(websocket.CloseMessage, closeMsg); err != nil {
		logger.Debug("write close message", "err", err)
	}

	if err := conn.Close(); err != nil {
		logger.Debug("close websocket", "err", err)
	}
}

func (s *Subscriptions) pipe(conn *websocket.Conn, reader msgReader, closed chan struct{}) error {
	ticker := s.repo.NewTicker()
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()
	for {
		msgs, hasMore, err := reader.Read()
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := conn.WriteJSON(msg); err != nil {
				return err
			}
		}
		if hasMore {
			select {
			case <-s.done:
				return nil
			case <-closed:
				return nil
			case <-pingTicker.C:
				conn.WriteMessage(websocket.PingMessage, nil)
			default:
			}
		} else {
			select {
			case <-s.done:
				return nil
			case <-closed:
				return nil
			case <-ticker.C():
			case <-pingTicker.C:
				conn.WriteMessage(websocket.PingMessage, nil)
			}
		}
	}
}

func (s *Subscriptions) parsePosition(posStr string) (thor.Bytes32, error) {
	bestID := s.repo.BestBlockSummary().Header.ID()
	if posStr == "" {
		return bestID, nil
	}
	pos, err := thor.ParseBytes32(posStr)
	if err != nil {
		return thor.Bytes32{}, utils.BadRequest(errors.WithMessage(err, "pos"))
	}
	if block.Number(bestID)-block.Number(pos) > s.backtraceLimit {
		return thor.Bytes32{}, utils.Forbidden(errors.New("pos: backtrace limit exceeded"))
	}
	return pos, nil
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

func (s *Subscriptions) websocket(readerFunc func(http.ResponseWriter, *http.Request) (msgReader, error)) utils.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) error {
		s.wg.Add(1)
		defer s.wg.Done()

		// Call the provided reader function
		reader, err := readerFunc(w, req)
		if err != nil {
			return err
		}

		// Setup WebSocket connection
		conn, closed, err := s.setupConn(w, req)
		if err != nil {
			logger.Debug("upgrade to websocket", "err", err)
			return err
		}
		defer s.closeConn(conn, err)

		// Stream messages
		err = s.pipe(conn, reader, closed)
		if err != nil {
			logger.Debug("error in websocket pipe", "err", err)
		}
		return err
	}
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/txpool").
		Methods(http.MethodGet).
		Name("WS /subscriptions/txpool"). // metrics middleware relies on this name
		HandlerFunc(utils.WrapHandlerFunc(s.handlePendingTransactions))

	sub.Path("/block").
		Methods(http.MethodGet).
		Name("WS /subscriptions/block"). // metrics middleware relies on this name
		HandlerFunc(utils.WrapHandlerFunc(s.websocket(s.handleBlockReader)))

	sub.Path("/event").
		Methods(http.MethodGet).
		Name("WS /subscriptions/event"). // metrics middleware relies on this name
		HandlerFunc(utils.WrapHandlerFunc(s.websocket(s.handleEventReader)))

	sub.Path("/transfer").
		Methods(http.MethodGet).
		Name("WS /subscriptions/transfer"). // metrics middleware relies on this name
		HandlerFunc(utils.WrapHandlerFunc(s.websocket(s.handleTransferReader)))

	sub.Path("/beat2").
		Methods(http.MethodGet).
		Name("WS /subscriptions/beat2"). // metrics middleware relies on this name
		HandlerFunc(utils.WrapHandlerFunc(s.websocket(s.handleBeat2Reader)))

	// This method is currently deprecated
	beatHandler := utils.HandleGone
	if s.enabledDeprecated {
		beatHandler = s.websocket(s.handleBeatReader)
	}
	sub.Path("/beat").
		Methods(http.MethodGet).
		Name("WS /subscriptions/beat"). // metrics middleware relies on this name
		HandlerFunc(utils.WrapHandlerFunc(beatHandler))
}
