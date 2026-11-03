// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

//go:build !windows

package subscriptions

import (
	"net"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// stalledDialer returns a websocket dialer whose sockets have a small pinned
// receive buffer, so the peer's writes block after a few messages. Callers
// must complete the handshake and then never read.
func stalledDialer() *websocket.Dialer {
	return &websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			d := &net.Dialer{
				Timeout: 5 * time.Second,
				Control: func(network, addr string, c syscall.RawConn) error {
					return c.Control(func(fd uintptr) {
						// Best effort; some kernels raise the value.
						_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4096)
					})
				},
			}
			return d.Dial(network, addr)
		},
	}
}
