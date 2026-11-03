// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// endlessReader feeds an unbounded stream of large messages so the handler's
// writes outrun any kernel buffer size.
type endlessReader struct {
	msg string
}

func (r endlessReader) Read() ([]any, bool, error) {
	return []any{map[string]string{"data": r.msg}}, true, nil
}

// TestWriteJSONFailsAgainstStalledPeer is the focused unit test of the
// bounded writer: a write to a peer that never reads must fail within
// writeWait instead of blocking forever.
func TestWriteJSONFailsAgainstStalledPeer(t *testing.T) {
	const wait = 300 * time.Millisecond

	type result struct {
		err     error
		elapsed time.Duration
	}
	writeDone := make(chan result, 1)

	upgrader := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			writeDone <- result{err: err}
			return
		}
		start := time.Now()
		msg := map[string]string{"data": strings.Repeat("x", 64*1024)}
		for {
			if err := writeJSON(conn, msg, wait); err != nil {
				writeDone <- result{err: err, elapsed: time.Since(start)}
				return
			}
		}
	}))
	defer ts.Close()

	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/"}
	conn, _, err := stalledDialer().Dial(u.String(), nil)
	require.NoError(t, err)
	defer conn.Close() // deliberately no reads before this point

	select {
	case res := <-writeDone:
		require.Error(t, res.err, "writeJSON must fail against a stalled peer")
		// The successful writes fill the buffers almost instantly, so the
		// failure must arrive at ~writeWait: a floor here catches a
		// writeJSON that errors immediately for the wrong reason.
		require.GreaterOrEqual(t, res.elapsed, wait-100*time.Millisecond,
			"writeJSON failed immediately (%s); the deadline was not the cause", res.elapsed)
		require.Less(t, res.elapsed, 2*time.Second,
			"writeJSON failed only after %s; the deadline did not bound the write", res.elapsed)
	case <-time.After(2 * time.Second):
		t.Fatal("writeJSON did not fail against a stalled peer within the budget")
	}
}

// TestStalledSubscriberDoesNotBlockClose pins the regression fixed by the
// bounded writer: a subscriber whose socket stops reading must fail the
// handler's blocked write within writeWait, release the connection, and let
// Subscriptions.Close() return. Before the fix the handler blocked inside
// the write forever (hijacked connections see no http.Server timeout and no
// Server.Close), and Close() hung on its WaitGroup.
func TestStalledSubscriberDoesNotBlockClose(t *testing.T) {
	thorChain, err := testchain.NewDefault()
	require.NoError(t, err)

	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	}, &thor.NoFork)

	sub := New(thorChain.Repo(), []string{}, 5, txPool, true)
	sub.writeWait = 300 * time.Millisecond

	// A subscription route whose reader never runs out of data.
	router := mux.NewRouter()
	router.Path("/subscriptions/stall").Methods(http.MethodGet).
		Handler(restutil.WrapHandlerFunc(sub.websocket(
			func(http.ResponseWriter, *http.Request) (msgReader, error) {
				return endlessReader{msg: strings.Repeat("x", 64*1024)}, nil
			})))
	ts := httptest.NewServer(router)
	defer ts.Close()

	// The attacker: a websocket client that completes the upgrade and then
	// never reads, with its receive buffer pinned small so the server's
	// writes block after a few messages.
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/stall"}
	conn, resp, err := stalledDialer().Dial(u.String(), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer conn.Close() // deliberately no reads before this point

	// The handler is now streaming into a socket nobody drains; its write
	// blocks after a few messages (buffers hold <450KB, each frame is 64KB).
	// Let the block engage before triggering shutdown: if Close() fires
	// first, the handler exits politely via the done channel and the test
	// would exercise nothing.
	stallStart := time.Now()
	time.Sleep(150 * time.Millisecond)

	closed := make(chan struct{})
	go func() {
		sub.Close()
		close(closed)
	}()
	select {
	case <-closed:
		// The handler's blocked write began around stallStart, so its
		// deadline fires writeWait later. If Close() returned well before
		// that, the handler was never blocked and exited politely via the
		// done channel - the test would be vacuous.
		total := time.Since(stallStart)
		require.GreaterOrEqual(t, total, sub.writeWait-50*time.Millisecond,
			"Close() returned before the write deadline could fire: the handler was never blocked")
		require.Less(t, total, 5*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("Subscriptions.Close() hangs on a stalled subscriber")
	}

	// The stalled connection must actually be released. closeConn runs
	// before the handler leaves the WaitGroup, so once Close() returns the
	// socket is server-side closed: draining the buffered frames must hit a
	// close frame or EOF, not a read deadline.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("stalled connection was not closed by the server: read deadline hit with buffered data remaining")
			}
			break // close frame or socket teardown observed
		}
	}
}

// TestCloseConnSendsCloseFrame covers the teardown half of the connection
// lifecycle: the peer must receive a proper close frame whose code reflects
// the error argument.
func TestCloseConnSendsCloseFrame(t *testing.T) {
	for name, tt := range map[string]struct {
		err      error
		wantCode int
		wantText string
	}{
		"going away":            {nil, websocket.CloseGoingAway, ""},
		"internal server error": {errors.New("boom"), websocket.CloseInternalServerErr, "boom"},
	} {
		t.Run(name, func(t *testing.T) {
			serverConnCh := make(chan *websocket.Conn, 1)
			release := make(chan struct{})
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					t.Errorf("upgrade: %v", err)
					return
				}
				serverConnCh <- conn
				<-release // hold the handler open so the connection stays live
			}))
			defer ts.Close()
			defer close(release)

			u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/"}
			client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			require.NoError(t, err)
			defer client.Close()

			serverConn := <-serverConnCh
			closeConn(serverConn, tt.err, time.Second)

			require.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))
			_, _, err = client.ReadMessage()
			var closeErr *websocket.CloseError
			require.True(t, errors.As(err, &closeErr), "expected a close frame, got %v", err)
			assert.Equal(t, tt.wantCode, closeErr.Code)
			assert.Equal(t, tt.wantText, closeErr.Text)
		})
	}
}

// upgradedPair returns an upgraded server/client websocket pair for direct
// lifecycle tests. The server handler blocks until release, holding the
// connection open.
func upgradedPair(t *testing.T) (serverConn *websocket.Conn, client *websocket.Conn, release func()) {
	t.Helper()
	serverConnCh := make(chan *websocket.Conn, 1)
	releaseCh := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConnCh <- conn
		<-releaseCh
	}))
	cleanup := func() {
		close(releaseCh)
		ts.Close()
	}

	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/"}
	client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	var server *websocket.Conn
	select {
	case server = <-serverConnCh:
	case <-time.After(3 * time.Second):
		cleanup()
		t.Fatal("server side never upgraded")
	}
	return server, client, cleanup
}

// TestPongWatchdogDropsSilentPeer pins the read watchdog: a peer that goes
// silent (no pongs - a client that never reads cannot answer pings) is given
// up on once pongWait elapses. A handler blocked in a write never observes
// the channel - that case is pinned by TestStalledSubscriberDoesNotBlockClose.
func TestPongWatchdogDropsSilentPeer(t *testing.T) {
	const pongWait = 300 * time.Millisecond

	serverConn, client, cleanup := upgradedPair(t)
	defer cleanup()
	defer client.Close()

	closed := make(chan struct{})
	watchdogDone := make(chan struct{})
	go func() {
		readWatchdog(serverConn, closed, pongWait)()
		close(watchdogDone)
	}()

	// The client never writes: the watchdog's read deadline is the only
	// thing that can fire. The floor proves the release came from the
	// watchdog rather than something immediate.
	start := time.Now()
	select {
	case <-closed:
		require.GreaterOrEqual(t, time.Since(start), pongWait-50*time.Millisecond,
			"watchdog fired before pongWait")
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog did not fire on a silent peer")
	}
	<-watchdogDone

	closeConn(serverConn, nil, time.Second)
	require.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, _, err := client.ReadMessage()
	var closeErr *websocket.CloseError
	require.True(t, errors.As(err, &closeErr), "expected a close frame, got %v", err)
	assert.Equal(t, websocket.CloseGoingAway, closeErr.Code)
}

// TestPongWatchdogRefreshesOnPong pins the watchdog's other half: pongs keep
// a connection alive past pongWait, and only silence ends it.
func TestPongWatchdogRefreshesOnPong(t *testing.T) {
	const pongWait = 400 * time.Millisecond

	serverConn, client, cleanup := upgradedPair(t)
	defer cleanup()
	defer client.Close()

	closed := make(chan struct{})
	go readWatchdog(serverConn, closed, pongWait)()

	// Pong well past pongWait: the watchdog must survive.
	keepAlive := time.After(2 * pongWait)
alive:
	for {
		select {
		case <-keepAlive:
			break alive
		case <-closed:
			t.Fatal("watchdog fired although the peer kept ponging")
		default:
		}
		require.NoError(t, client.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second)))
		time.Sleep(100 * time.Millisecond)
	}

	// Stop ponging: the refreshed deadline expires within pongWait.
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not fire after the peer went silent")
	}
	closeConn(serverConn, nil, time.Second)
}
