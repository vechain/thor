// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"time"

	"github.com/gorilla/websocket"
)

// Hijacked-connection lifecycle, stateless half: read watchdog, bounded
// writes, teardown. Nothing in this file knows about Subscriptions - the
// wiring (upgrade policy, WaitGroup registration, timing values) lives in
// subscriptions.go. A hijacked connection is invisible to http.Server
// timeouts and to Server.Close; its release must come from this file.

// readWatchdog returns the read-side watchdog body for a hijacked
// connection: it closes closed once the peer goes silent past pongWait or
// the connection fails. The body is returned unspawned so the owner can
// register it where shutdown will wait for it.
func readWatchdog(conn *websocket.Conn, closed chan struct{}, pongWait time.Duration) func() {
	return func() {
		defer close(closed)

		conn.SetReadLimit(100 * 1024) // 100 KB
		if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			logger.Debug("failed to set initial read deadline", "err", err)
			return
		}
		conn.SetPongHandler(func(string) error {
			if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
				logger.Debug("failed to set pong read deadline", "err", err)
			}
			return nil
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				logger.Debug("websocket read err", "err", err)
				break
			}
		}
	}
}

// closeConn sends a best-effort close frame (bounded by writeWait) and
// closes the connection.
func closeConn(conn *websocket.Conn, err error, writeWait time.Duration) {
	var closeMsg []byte
	if err != nil {
		closeMsg = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error())
	} else {
		closeMsg = websocket.FormatCloseMessage(websocket.CloseGoingAway, "")
	}

	if err := writeMessage(conn, websocket.CloseMessage, closeMsg, writeWait); err != nil {
		logger.Debug("write close message", "err", err)
	}

	if err := conn.Close(); err != nil {
		logger.Debug("close websocket", "err", err)
	}
}

// writeJSON writes one JSON frame bounded by writeWait.
func writeJSON(conn *websocket.Conn, v any, writeWait time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return conn.WriteJSON(v)
}

// writeMessage writes one frame (pings, the close frame) bounded by
// writeWait.
func writeMessage(conn *websocket.Conn, messageType int, data []byte, writeWait time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, data)
}
