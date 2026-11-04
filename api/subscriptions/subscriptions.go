// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
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
	beat2Cache        *messageCache[api.Beat2Message]
	beatCache         *messageCache[api.BeatMessage]

	// pongWait is the read-watchdog silence budget, refreshed on every pong.
	pongWait time.Duration
	// writeWait is the per-frame write deadline for every websocket write.
	writeWait time.Duration
}

type msgReader interface {
	Read() (msgs []any, hasMore bool, err error)
}

var logger = log.WithContext("pkg", "subscriptions")

const (
	defaultPongWait = 60 * time.Second
)

func New(
	repo *chain.Repository,
	allowedOrigins []string,
	backtraceLimit uint32,
	txpool txpool.Pool,
	enabledDeprecated bool,
	writeWait time.Duration,
) *Subscriptions {
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
		beat2Cache: newMessageCache[api.Beat2Message](backtraceLimit),
		beatCache:  newMessageCache[api.BeatMessage](backtraceLimit),
		pongWait:   defaultPongWait,
		writeWait:  writeWait,
	}
	sub.wg.Go(func() {
		sub.pendingTx.DispatchLoop(sub.done)
	})

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
		return nil, restutil.BadRequest(errors.WithMessage(err, "addr"))
	}
	t0, err := parseTopic(req.URL.Query().Get("t0"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "t0"))
	}
	t1, err := parseTopic(req.URL.Query().Get("t1"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "t1"))
	}
	t2, err := parseTopic(req.URL.Query().Get("t2"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "t2"))
	}
	t3, err := parseTopic(req.URL.Query().Get("t3"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "t3"))
	}
	t4, err := parseTopic(req.URL.Query().Get("t4"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "t4"))
	}
	eventFilter := &api.SubscriptionEventFilter{
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
		return nil, restutil.BadRequest(errors.WithMessage(err, "txOrigin"))
	}
	sender, err := parseAddress(req.URL.Query().Get("sender"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "sender"))
	}
	recipient, err := parseAddress(req.URL.Query().Get("recipient"))
	if err != nil {
		return nil, restutil.BadRequest(errors.WithMessage(err, "recipient"))
	}
	transferFilter := &api.SubscriptionTransferFilter{
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
		// websocket connection do not return errors to the wrapHandler
		return nil
	}
	defer closeConn(conn, err, s.writeWait)

	pingTicker := time.NewTicker(s.pingPeriod())
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
			if err = writeJSON(conn, &api.PendingTxIDMessage{ID: tx.ID()}, s.writeWait); err != nil {
				// likely conn has failed
				return nil
			}
		case <-s.done:
			return nil
		case <-closed:
			return nil
		case <-pingTicker.C:
			if err = writeMessage(conn, websocket.PingMessage, nil, s.writeWait); err != nil {
				// likely conn has failed
				return nil
			}
		}
	}
}

// setupConn upgrades the request and starts the read watchdog. The returned
// channel closes when the watchdog gives up on the peer (read deadline
// expired or connection failed); a handler blocked in a write never observes
// it. The watchdog runs on the service's WaitGroup: Close() waits for it,
// and closeConn's socket teardown unblocks its pending read.
func (s *Subscriptions) setupConn(w http.ResponseWriter, req *http.Request) (*websocket.Conn, chan struct{}, error) {
	conn, err := s.upgrader.Upgrade(w, req, nil)
	if err != nil {
		return nil, nil, err
	}

	closed := make(chan struct{})
	s.wg.Go(readWatchdog(conn, closed, s.pongWait))
	return conn, closed, nil
}

// pingPeriod returns the ping interval, kept below pongWait (7/10) so pings
// arrive before the watchdog expires.
func (s *Subscriptions) pingPeriod() time.Duration {
	return s.pongWait * 7 / 10
}

func (s *Subscriptions) pipe(conn *websocket.Conn, reader msgReader, closed chan struct{}) error {
	ticker := s.repo.NewTicker()
	pingTicker := time.NewTicker(s.pingPeriod())
	defer pingTicker.Stop()
	for {
		msgs, hasMore, err := reader.Read()
		if err != nil {
			return fmt.Errorf("unable to read subscription message: %w", err)
		}
		for _, msg := range msgs {
			if err := writeJSON(conn, msg, s.writeWait); err != nil {
				return fmt.Errorf("unable to write subscription json: %w", err)
			}
		}
		if hasMore {
			select {
			case <-s.done:
				return nil
			case <-closed:
				return nil
			case <-pingTicker.C:
				if err = writeMessage(conn, websocket.PingMessage, nil, s.writeWait); err != nil {
					return fmt.Errorf("failed to write ping message: %w", err)
				}
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
				if err = writeMessage(conn, websocket.PingMessage, nil, s.writeWait); err != nil {
					return fmt.Errorf("failed to write ping message: %w", err)
				}
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
		return thor.Bytes32{}, restutil.BadRequest(errors.WithMessage(err, "pos"))
	}
	if block.Number(bestID)-block.Number(pos) > s.backtraceLimit {
		return thor.Bytes32{}, restutil.Forbidden(errors.New("pos: backtrace limit exceeded"))
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

func (s *Subscriptions) websocket(readerFunc func(http.ResponseWriter, *http.Request) (msgReader, error)) restutil.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) error {
		s.wg.Add(1)
		defer s.wg.Done()

		// Call the provided reader function
		reader, err := readerFunc(w, req)
		if err != nil {
			// it's not yet a websocket connection, this is likely a setup error in the original http request
			return err
		}

		// Setup WebSocket connection
		conn, closed, err := s.setupConn(w, req)
		if err != nil {
			logger.Debug("upgrade to websocket", "err", err)
			// websocket connection do not return errors to the wrapHandler
			return nil
		}
		defer closeConn(conn, err, s.writeWait)

		// Stream messages
		err = s.pipe(conn, reader, closed)
		if err != nil {
			logger.Debug("error in websocket pipe", "err", err)
			// websocket connection do not return errors to the wrapHandler
		}
		return nil
	}
}

func (s *Subscriptions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/txpool").
		Methods(http.MethodGet).
		Name("WS /subscriptions/txpool"). // metrics middleware relies on this name
		HandlerFunc(restutil.WrapHandlerFunc(s.handlePendingTransactions))

	sub.Path("/block").
		Methods(http.MethodGet).
		Name("WS /subscriptions/block"). // metrics middleware relies on this name
		HandlerFunc(restutil.WrapHandlerFunc(s.websocket(s.handleBlockReader)))

	sub.Path("/event").
		Methods(http.MethodGet).
		Name("WS /subscriptions/event"). // metrics middleware relies on this name
		HandlerFunc(restutil.WrapHandlerFunc(s.websocket(s.handleEventReader)))

	sub.Path("/transfer").
		Methods(http.MethodGet).
		Name("WS /subscriptions/transfer"). // metrics middleware relies on this name
		HandlerFunc(restutil.WrapHandlerFunc(s.websocket(s.handleTransferReader)))

	sub.Path("/beat2").
		Methods(http.MethodGet).
		Name("WS /subscriptions/beat2"). // metrics middleware relies on this name
		HandlerFunc(restutil.WrapHandlerFunc(s.websocket(s.handleBeat2Reader)))

	// This method is currently deprecated
	beatHandler := restutil.HandleGone
	if s.enabledDeprecated {
		beatHandler = s.websocket(s.handleBeatReader)
	}
	sub.Path("/beat").
		Methods(http.MethodGet).
		Name("WS /subscriptions/beat"). // metrics middleware relies on this name
		HandlerFunc(restutil.WrapHandlerFunc(beatHandler))
}
