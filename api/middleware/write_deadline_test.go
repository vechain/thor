// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package middleware

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smallBufferListener pins each accepted connection's send buffer, so a peer that
// reads slowly or not at all blocks the server's writes after little data.
type smallBufferListener struct {
	net.Listener
}

func (l smallBufferListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		_ = c.(*net.TCPConn).SetWriteBuffer(16 * 1024)
	}
	return c, err
}

func newWriteDeadlineServer(t *testing.T, timeout time.Duration, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(HandleWriteDeadline(timeout)(h))
	ts.Listener = smallBufferListener{ts.Listener}
	ts.Start()
	t.Cleanup(ts.Close)
	return ts
}

// dialSmallBuffer opens a raw connection with a pinned receive buffer and sends a
// GET for path.
func dialSmallBuffer(t *testing.T, ts *httptest.Server, path string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	_ = conn.(*net.TCPConn).SetReadBuffer(4 * 1024)
	_, err = io.WriteString(conn, "GET "+path+" HTTP/1.1\r\nHost: test\r\n\r\n")
	require.NoError(t, err)
	return conn
}

func TestHandleWriteDeadlineCutsOffStalledReader(t *testing.T) {
	const timeout = 300 * time.Millisecond

	type result struct {
		err     error
		elapsed time.Duration
	}
	done := make(chan result, 1)
	ts := newWriteDeadlineServer(t, timeout, func(w http.ResponseWriter, _ *http.Request) {
		start := time.Now()
		chunk := bytes.Repeat([]byte("x"), 1024*1024)
		for range 64 {
			if _, err := w.Write(chunk); err != nil {
				done <- result{err, time.Since(start)}
				return
			}
		}
		done <- result{nil, time.Since(start)}
	})

	dialSmallBuffer(t, ts, "/") // never read

	select {
	case res := <-done:
		require.Error(t, res.err, "a write to a peer that never reads must fail")
		assert.Less(t, res.elapsed, 3*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("handler write still blocked on a stalled reader")
	}
}

// slowConn stands in for the connection under a ResponseWriter: it drains rate
// bytes per second, and a write that finishes past its deadline fails, as on a
// net.Conn. Unlike a real socket, its pace does not depend on the platform's TCP
// buffering.
type slowConn struct {
	header   http.Header
	rate     int // bytes per second
	deadline time.Time
	written  int
}

func (c *slowConn) Header() http.Header { return c.header }

func (c *slowConn) WriteHeader(int) {}

func (c *slowConn) Write(b []byte) (int, error) {
	time.Sleep(time.Duration(len(b)) * time.Second / time.Duration(c.rate))
	if !c.deadline.IsZero() && time.Now().After(c.deadline) {
		return 0, os.ErrDeadlineExceeded
	}
	c.written += len(b)
	return len(b), nil
}

func (c *slowConn) SetWriteDeadline(t time.Time) error {
	c.deadline = t
	return nil
}

// TestHandleWriteDeadlineServesSlowReader pins the per-chunk deadline: the response
// takes several times the timeout to deliver, so it only completes if the deadline
// moves with each chunk instead of bounding the whole write.
func TestHandleWriteDeadlineServesSlowReader(t *testing.T) {
	const (
		timeout = 100 * time.Millisecond
		size    = 1024 * 1024
	)
	// One chunk drains in a fifth of the timeout; the whole body needs several.
	conn := &slowConn{header: http.Header{}, rate: writeChunkSize * 5 * int(time.Second/timeout)}

	var writeErr error
	start := time.Now()
	HandleWriteDeadline(timeout)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr = w.Write(bytes.Repeat([]byte("x"), size)) // one call, as WriteJSON makes
	})).ServeHTTP(conn, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Greater(t, time.Since(start), 2*timeout, "the write was not slow enough to exercise the deadline")
	require.NoError(t, writeErr)
	assert.Equal(t, size, conn.written)
}

// TestHandleWriteDeadlineArmedByFirstWrite pins that time before the response starts
// is not charged to the write deadline, and that a connection reused after a
// deadline-bounded response serves the next request normally.
func TestHandleWriteDeadlineArmedByFirstWrite(t *testing.T) {
	const timeout = 100 * time.Millisecond

	ts := newWriteDeadlineServer(t, timeout, func(w http.ResponseWriter, r *http.Request) {
		body := "fast"
		if r.URL.Path == "/slow" {
			time.Sleep(3 * timeout)
			body = "slow"
		}
		_, _ = io.WriteString(w, body)
	})

	get := func(path string) (string, bool) {
		var reused bool
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+path, nil)
		require.NoError(t, err)
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(b), reused
	}

	body, _ := get("/fast")
	assert.Equal(t, "fast", body)

	time.Sleep(3 * timeout) // the deadline armed by /fast is now in the past
	body, reused := get("/slow")
	assert.Equal(t, "slow", body)
	assert.True(t, reused, "the second request should reuse the connection")
}

func TestHandleWriteDeadlinePassesWebsocketUpgrade(t *testing.T) {
	ts := httptest.NewServer(HandleWriteDeadline(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("hello"))
	})))
	defer ts.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "hello", string(msg))
}
