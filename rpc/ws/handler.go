// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package ws implements Ethereum eth_subscribe / eth_unsubscribe over WebSocket.
// The Handler wraps an existing jsonrpc.Server: plain HTTP POST requests are
// forwarded to it unchanged; WebSocket upgrade requests are served here with
// push-based subscriptions multiplexed on the same connection.
package ws

import (
	"context"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/txpool"
)

const (
	pongWait     = 60  // seconds — read deadline after each pong
	pingPeriod   = 42  // seconds — ping interval (7/10 of pongWait)
	writeTimeout = 10  // seconds — per-write deadline for data frames; connection is closed on expiry
	writeBufSize = 256 // per-connection notification buffer; full buffer triggers disconnect
)

type Syncer interface {
	Synced() <-chan struct{}

	HighestPeerBlock() uint32
}

// Handler is an http.Handler that serves JSON-RPC over both HTTP and WebSocket
// at the same endpoint. HTTP POST requests are forwarded to rpcSrv; WebSocket
// connections gain eth_subscribe / eth_unsubscribe in addition to all registered
// methods.
type Handler struct {
	repo     *chain.Repository
	txPool   txpool.Pool
	rpcSrv   *jsonrpc.Server
	syncer   Syncer
	upgrader *websocket.Upgrader

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a Handler. allowedOrigins controls the WebSocket CORS check;
// pass the same slice used for the REST API. syncer powers the "syncing"
// subscription type; pass nil only in contexts where that subscription must
// be rejected.
func New(repo *chain.Repository, txPool txpool.Pool, allowedOrigins []string, rpcSrv *jsonrpc.Server, syncer Syncer) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Handler{
		repo:   repo,
		txPool: txPool,
		rpcSrv: rpcSrv,
		syncer: syncer,
		upgrader: &websocket.Upgrader{
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				for _, allowed := range allowedOrigins {
					if allowed == origin || allowed == "*" {
						return true
					}
				}
				return false
			},
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// ServeHTTP dispatches WebSocket upgrade requests to the subscription handler
// and all other requests to the underlying jsonrpc.Server.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		h.serveWS(w, r)
		return
	}
	h.rpcSrv.ServeHTTP(w, r)
}

// Close stops all active WebSocket connections gracefully and waits for them
// to finish. It should be called during server shutdown.
func (h *Handler) Close() {
	h.cancel()
	h.wg.Wait()
}

func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade writes the error response; nothing more to do.
		return
	}

	h.wg.Go(func() {
		c := newWSConn(conn, h.ctx, h.repo, h.txPool, h.rpcSrv, h.syncer)
		c.serve()
	})
}
