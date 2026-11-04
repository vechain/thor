// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"bytes"
	"context"
	"errors"
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

const testWriteTimeout = 300 * time.Millisecond

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

func newDeadlineServer(t *testing.T, timeout time.Duration, h http.Handler) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(h)
	ts.Listener = writeDeadlineListener{smallBufferListener{ts.Listener}, timeout}
	ts.Config.ConnState = markHijacked
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

func TestWriteDeadlineCutsOffStalledReader(t *testing.T) {
	type result struct {
		err     error
		elapsed time.Duration
	}
	done := make(chan result, 1)
	ts := newDeadlineServer(t, testWriteTimeout, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		start := time.Now()
		chunk := bytes.Repeat([]byte("x"), 1024*1024)
		for range 64 {
			if _, err := w.Write(chunk); err != nil {
				done <- result{err, time.Since(start)}
				return
			}
		}
		done <- result{nil, time.Since(start)}
	}))

	dialSmallBuffer(t, ts, "/") // never read

	select {
	case res := <-done:
		require.Error(t, res.err, "a write to a peer that never reads must fail")
		assert.Less(t, res.elapsed, 3*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("handler write still blocked on a stalled reader")
	}
}

// slowConn stands in for a connection: it drains rate bytes per second, and a
// write that finishes past its deadline fails, as on a net.Conn. Unlike a real
// socket, its pace does not depend on the platform's TCP buffering.
type slowConn struct {
	rate     int // bytes per second
	deadline time.Time
	written  int
}

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

func (c *slowConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (c *slowConn) Close() error                    { return nil }
func (c *slowConn) LocalAddr() net.Addr             { return nil }
func (c *slowConn) RemoteAddr() net.Addr            { return nil }
func (c *slowConn) SetDeadline(time.Time) error     { return nil }
func (c *slowConn) SetReadDeadline(time.Time) error { return nil }

// TestWriteDeadlineServesSlowReader pins the per-chunk deadline: the write takes
// several times the timeout to deliver, so it only completes if the deadline
// moves with each chunk instead of bounding the whole write.
func TestWriteDeadlineServesSlowReader(t *testing.T) {
	const (
		timeout = 100 * time.Millisecond
		size    = 1024 * 1024
	)
	// One chunk drains in a fifth of the timeout; the whole body needs several.
	slow := &slowConn{rate: writeChunkSize * 5 * int(time.Second/timeout)}
	c := &writeDeadlineConn{Conn: slow, timeout: timeout}

	start := time.Now()
	n, err := c.Write(bytes.Repeat([]byte("x"), size))

	require.Greater(t, time.Since(start), 2*timeout, "the write was not slow enough to exercise the deadline")
	require.NoError(t, err)
	assert.Equal(t, size, n)
	assert.Equal(t, size, slow.written)
}

// TestWriteDeadlineIdleBeforeWrite pins that time before the response starts is
// not charged to the write deadline, and that the connection is reused after.
func TestWriteDeadlineIdleBeforeWrite(t *testing.T) {
	timeout := testWriteTimeout
	ts := newDeadlineServer(t, timeout, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "fast"
		if r.URL.Path == "/slow" {
			time.Sleep(3 * timeout)
			body = "slow"
		}
		_, _ = io.WriteString(w, body)
	}))

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

// TestWriteDeadlineSkipsHijackedConn pins that after a hijack the wrapper does not
// re-arm the deadline, so the handler's own (shorter) deadline holds.
func TestWriteDeadlineSkipsHijackedConn(t *testing.T) {
	const handlerDeadline = 50 * time.Millisecond
	type result struct {
		err error
		set time.Time
		end time.Time
	}
	done := make(chan result, 1)
	ts := newDeadlineServer(t, testWriteTimeout, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(2 * testWriteTimeout)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("hello"))

		set := time.Now()
		_ = conn.SetWriteDeadline(set.Add(handlerDeadline))
		msg := bytes.Repeat([]byte("x"), 1024*1024)
		for {
			if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				done <- result{err, set, time.Now()}
				return
			}
		}
	}))

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "hello", string(msg)) // then stop reading

	select {
	case res := <-done:
		assert.Less(t, res.end.Sub(res.set), testWriteTimeout, "the wrapper re-armed the deadline after hijack")
		// gorilla re-wraps temporary net errors, so errors.Is(os.ErrDeadlineExceeded) no longer matches.
		var ne net.Error
		require.True(t, errors.As(res.err, &ne), "got %v", res.err)
		assert.True(t, ne.Timeout(), "got %v", res.err)
	case <-time.After(10 * time.Second):
		t.Fatal("handler write still blocked on a stalled websocket reader")
	}
}

func TestWriteDeadlineCloseWriteForwarded(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	require.NoError(t, (&writeDeadlineConn{Conn: c1}).CloseWrite())

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := l.Accept()
		accepted <- c
	}()
	client, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	server := <-accepted
	defer server.Close()

	require.NoError(t, (&writeDeadlineConn{Conn: server.(*net.TCPConn)}).CloseWrite())
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = client.Read(make([]byte, 1))
	assert.Equal(t, io.EOF, err)
}

// io.ReaderFrom would let net/http's response.ReadFrom bypass per-chunk re-arming.
func TestWriteDeadlineConnHasNoReadFrom(t *testing.T) {
	_, ok := any(&writeDeadlineConn{}).(io.ReaderFrom)
	assert.False(t, ok)
}
