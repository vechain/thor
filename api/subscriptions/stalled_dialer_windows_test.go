// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

//go:build windows

package subscriptions

import (
	"github.com/gorilla/websocket"
)

// stalledDialer is the Windows variant: no receive-buffer pin. The Unix
// version shrinks how much data the peer can queue before blocking, but no
// test relies on the pin - the endless 64KB message streams fill any buffer
// - and the Windows syscall API takes a Handle, which the stdlib socket
// helpers here do not abstract.
func stalledDialer() *websocket.Dialer {
	return &websocket.Dialer{}
}
