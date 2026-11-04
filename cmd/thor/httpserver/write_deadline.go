// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

// This file bounds stalled response writes at the net.Conn: every chunk handed to
// the connection must be accepted within the timeout, so a client that stops
// reading is cut off while a slow one still receives a large response.
//
// Renewal granularity is the smaller of two sizes. Small writes (gzip's flate
// writes about every 240 bytes) are collected by net/http's 4KB connection buffer
// and reach the conn about every 4KB. Large uncompressed writes (WriteJSON in one
// piece, WriteJSONArray every 16KB) bypass that buffer, and Write splits them
// into writeChunkSize pieces.
//
// The conn must not implement or forward ReadFrom. net/http's response.ReadFrom
// would pass the data straight to it and skip the per-chunk renewal. The cost,
// no sendfile, is nothing today: /doc serves an embedded http.FS, which cannot
// use sendfile anyway.
//
// CloseWrite is forwarded because net/http half-closes the conn through it after
// an oversized request body. Without it the conn is closed instead, and the
// client may not see the error response.
//
// After a hijack (websocket) the conn passes writes through. The deadline there
// belongs to subscriptions' writeWait, which covers a whole message: re-arming it
// per frame would let a slow reader hold a large message for hours. The value is
// the same, the meaning differs: per chunk here, per message there.

import (
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const writeChunkSize = 32 * 1024

// withWriteDeadline returns l wrapped so that each write to an accepted conn is
// bounded by timeout, and installs on srv the ConnState hook that ends the bound
// at hijack; a ConnState hook already set on srv still runs. Without the hook, a
// hijacked conn would keep re-arming per chunk and override writeWait.
func withWriteDeadline(srv *http.Server, l net.Listener, timeout time.Duration) net.Listener {
	prev := srv.ConnState
	srv.ConnState = func(c net.Conn, s http.ConnState) {
		markHijacked(c, s)
		if prev != nil {
			prev(c, s)
		}
	}
	return writeDeadlineListener{l, timeout}
}

type writeDeadlineListener struct {
	net.Listener
	timeout time.Duration
}

func (l writeDeadlineListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &writeDeadlineConn{Conn: c, timeout: l.timeout}, nil
}

type writeDeadlineConn struct {
	net.Conn
	timeout  time.Duration
	hijacked atomic.Bool // set by markHijacked; later writes may come from other goroutines
}

func (c *writeDeadlineConn) Write(b []byte) (int, error) {
	if c.hijacked.Load() {
		return c.Conn.Write(b)
	}
	var n int
	for len(b) > 0 {
		if err := c.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
			return n, err
		}
		m, err := c.Conn.Write(b[:min(len(b), writeChunkSize)])
		n += m
		if err != nil {
			return n, err
		}
		b = b[m:]
	}
	return n, nil
}

func (c *writeDeadlineConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// markHijacked is the ConnState hook that withWriteDeadline installs. net/http
// reports StateHijacked before Hijack returns, and only for a real Hijack call,
// so a request header cannot trigger it.
func markHijacked(c net.Conn, s http.ConnState) {
	if s != http.StateHijacked {
		return
	}
	if wc, ok := c.(*writeDeadlineConn); ok {
		wc.hijacked.Store(true)
	}
}
