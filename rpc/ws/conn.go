// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/websocket"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/txpool"
)

// notification is the JSON-RPC push envelope sent for each subscription event.
// It has no "id" field, distinguishing it from a response to a request.
type notification struct {
	Jsonrpc string             `json:"jsonrpc"`
	Method  string             `json:"method"`
	Params  notificationParams `json:"params"`
}

type notificationParams struct {
	Subscription string          `json:"subscription"`
	Result       json.RawMessage `json:"result"`
}

// wsConn manages the lifecycle of a single WebSocket connection: one read loop,
// one write loop, and N subscription goroutines (one per active eth_subscribe call).
type wsConn struct {
	conn    *websocket.Conn
	writeCh chan []byte // pre-serialised JSON frames; closed only after all writers exit

	// connCtx is cancelled on client disconnect or server shutdown.
	connCtx    context.Context
	connCancel context.CancelFunc

	repo   *chain.Repository
	txPool txpool.Pool
	rpcSrv *jsonrpc.Server
	syncer Syncer

	subsMu  sync.Mutex
	subs    map[string]context.CancelFunc // subID → cancel for that sub's goroutine
	nextSub atomic.Uint64
	subWg   sync.WaitGroup
}

func newWSConn(conn *websocket.Conn, parentCtx context.Context, repo *chain.Repository, txPool txpool.Pool, rpcSrv *jsonrpc.Server, syncer Syncer) *wsConn {
	ctx, cancel := context.WithCancel(parentCtx)
	return &wsConn{
		conn:       conn,
		writeCh:    make(chan []byte, writeBufSize),
		connCtx:    ctx,
		connCancel: cancel,
		repo:       repo,
		txPool:     txPool,
		rpcSrv:     rpcSrv,
		syncer:     syncer,
		subs:       make(map[string]context.CancelFunc),
	}
}

// serve runs the read and write loops, blocking until the connection closes.
func (c *wsConn) serve() {
	defer func() {
		// connCancel stops subscription goroutines; subWg.Wait ensures none
		// outlive serve(), so that the caller's wg.Done fires only after full cleanup.
		c.connCancel()
		c.subWg.Wait()
	}()

	c.conn.SetReadLimit(100 * 1024) // 100 KB per frame

	// Pong handler: reset the read deadline each time the peer responds to a ping,
	// keeping the connection alive as long as the client is reachable.
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait * time.Second)); err != nil {
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	})

	// Close the underlying connection when connCtx is cancelled (server shutdown
	// or explicit client teardown). This unblocks the blocking ReadMessage call
	// in readLoop so serve() can return promptly without goroutine leaks.
	go func() {
		<-c.connCtx.Done()
		c.conn.Close()
	}()

	var writeWg sync.WaitGroup
	writeWg.Go(func() {
		c.writeLoop()
	})

	c.readLoop()   // blocks until conn.Close() or read error
	c.connCancel() // stop write loop and all subscription goroutines
	writeWg.Wait() // wait for write loop to drain and exit
}

// readLoop reads JSON-RPC frames from the client and dispatches them.
// It exits when the connection is closed or a read error occurs (including
// the pongWait deadline expiring after a missed pong).
func (c *wsConn) readLoop() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.dispatch(msg)
	}
}

// dispatch parses one frame (single or batch) and routes it.
func (c *wsConn) dispatch(msg []byte) {
	trimmed := bytes.TrimSpace(msg)
	if len(trimmed) == 0 {
		return
	}

	if trimmed[0] == '[' {
		// Batch request.
		// TODO: enforce a batch size cap here (the HTTP path uses jsonrpc.maxBatchRequests=10).
		// WS batch requests currently have no size limit — a single frame can carry thousands
		// of requests, all dispatched synchronously in the read goroutine.
		var raws []json.RawMessage
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			c.send(mustMarshal(jsonrpc.ErrResponse(nil, jsonrpc.CodeParseError, "invalid JSON array: "+err.Error())))
			return
		}
		responses := make([]jsonrpc.Response, len(raws))
		for i, raw := range raws {
			responses[i] = c.dispatchOne(raw)
		}
		c.send(mustMarshal(responses))
	} else {
		resp := c.dispatchOne(trimmed)
		c.send(mustMarshal(resp))
	}
}

func (c *wsConn) dispatchOne(raw []byte) jsonrpc.Response {
	var req jsonrpc.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return jsonrpc.ErrResponse(nil, jsonrpc.CodeParseError, "invalid JSON: "+err.Error())
	}
	switch req.Method {
	case "eth_subscribe":
		return c.subscribe(req)
	case "eth_unsubscribe":
		return c.unsubscribe(req)
	default:
		return c.rpcSrv.Dispatch(req)
	}
}

// subscribe handles eth_subscribe: spawns the appropriate subscription goroutine
// and returns the subscription ID.
func (c *wsConn) subscribe(req jsonrpc.Request) jsonrpc.Response {
	var params []json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [subscriptionType, ...]")
	}
	var subType string
	if err := json.Unmarshal(params[0], &subType); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid subscription type")
	}

	subID := hexutil.EncodeUint64(c.nextSub.Add(1))

	switch subType {
	case "newHeads":
		c.startSub(subID, func(ctx context.Context) {
			runNewHeads(ctx, c, subID)
		})
	case "logs":
		var filter rpc.EthLogFilter
		if len(params) > 1 {
			if err := json.Unmarshal(params[1], &filter); err != nil {
				return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid logs filter: "+err.Error())
			}
		}
		criteria, err := ethconvert.ParseLogCriteria(filter)
		if err != nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
		}
		c.startSub(subID, func(ctx context.Context) {
			runLogs(ctx, c, subID, criteria)
		})
	case "newPendingTransactions":
		c.startSub(subID, func(ctx context.Context) {
			runNewPendingTransactions(ctx, c, subID)
		})
	case "syncing":
		if c.syncer == nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "syncing subscription not available")
		}
		startBlock := c.repo.BestBlockSummary().Header.Number()
		c.startSub(subID, func(ctx context.Context) {
			runSyncing(ctx, c, subID, startBlock)
		})
	default:
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, fmt.Sprintf("unsupported subscription type %q", subType))
	}

	return jsonrpc.OkResponse(req.ID, subID)
}

// startSub registers a subscription and runs fn in a goroutine.
// The goroutine is tracked in subWg so serve() can wait for all of them.
// TODO: add a per-connection subscription cap to prevent goroutine exhaustion.
// A client can call eth_subscribe unlimited times; each call spawns a goroutine that
// lives until the connection closes. Decide the right cap value before implementing.
func (c *wsConn) startSub(subID string, fn func(context.Context)) {
	ctx, cancel := context.WithCancel(c.connCtx)
	c.subsMu.Lock()
	c.subs[subID] = cancel
	c.subsMu.Unlock()

	c.subWg.Go(func() {
		defer func() {
			c.subsMu.Lock()
			delete(c.subs, subID)
			c.subsMu.Unlock()
			cancel()
		}()
		fn(ctx)
	})
}

// unsubscribe handles eth_unsubscribe: cancels the subscription goroutine.
func (c *wsConn) unsubscribe(req jsonrpc.Request) jsonrpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [subscriptionId]")
	}
	var subID string
	if err := json.Unmarshal(params[0], &subID); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid subscription id")
	}

	c.subsMu.Lock()
	cancel, ok := c.subs[subID]
	if ok {
		delete(c.subs, subID)
	}
	c.subsMu.Unlock()
	if ok {
		cancel()
	}
	return jsonrpc.OkResponse(req.ID, ok)
}

// writeLoop drains writeCh and sends frames to the client. It also sends
// periodic pings so the pong handler can reset the read deadline on the other
// side, keeping the connection alive through idle periods.
//
// A per-write deadline enforces the disconnect-on-slow-client policy: if the
// client is not consuming frames fast enough the write times out and
// connCancel() closes the connection.
func (c *wsConn) writeLoop() {
	pingTicker := time.NewTicker(pingPeriod * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case data := <-c.writeCh:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout * time.Second)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := c.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeTimeout*time.Second),
			); err != nil {
				return
			}
		case <-c.connCtx.Done():
			return
		}
	}
}

// send queues a pre-serialised frame for the write loop. If the buffer is full
// the connection is disconnected: the client is not reading fast enough.
func (c *wsConn) send(data []byte) {
	select {
	case c.writeCh <- data:
	case <-c.connCtx.Done():
	default:
		// Buffer full — disconnect the slow client.
		c.connCancel()
	}
}

// notify builds and queues a subscription notification frame.
func (c *wsConn) notify(subID string, result any) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return
	}
	data, err := json.Marshal(notification{
		Jsonrpc: "2.0",
		Method:  "eth_subscription",
		Params:  notificationParams{Subscription: subID, Result: resultBytes},
	})
	if err != nil {
		return
	}
	c.send(data)
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("ws: json.Marshal failed: " + err.Error())
	}
	return b
}
