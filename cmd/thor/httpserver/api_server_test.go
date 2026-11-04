// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

type noPeers struct{}

func (noPeers) PeersStats() []*comm.PeerStats { return nil }

// pipelineAndStall sends req n times on one connection, waits for stall, and
// then counts the responses. Less than n shows that the server closed the
// connection.
func pipelineAndStall(t *testing.T, addr, req string, n int, stall time.Duration) int {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()
	_ = conn.(*net.TCPConn).SetReadBuffer(4 * 1024)
	go func() { _, _ = io.WriteString(conn, strings.Repeat(req, n)) }()

	time.Sleep(stall)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(conn)
	got := 0
	for ; got < n; got++ {
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			break
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return got
}

// startTestAPIServer starts StartAPIServer on a testchain and returns its host:port.
func startTestAPIServer(t *testing.T, cfg APIConfig) string {
	t.Helper()
	chain, err := testchain.NewDefault()
	require.NoError(t, err)
	pool := txpool.New(chain.Repo(), chain.Stater(), txpool.Options{
		Limit: 100, LimitPerAccount: 16, MaxLifetime: time.Minute,
	}, &thor.NoFork)
	t.Cleanup(pool.Close)

	cfg.AllowedOrigins = "*"
	cfg.BacktraceLimit = 1000
	cfg.CallGasLimit = 50_000_000
	cfg.BatchDataMaxSize = 1 << 20
	cfg.TraceDataMaxSize = 1 << 20
	cfg.EnableReqLogger = &atomic.Bool{}
	cfg.LogsLimit = 1000
	cfg.EnableTxPool = &atomic.Bool{}
	cfg.Timeout = 10_000
	cfg.MaxLogsOffset = 100_000
	url, closeSrv, err := StartAPIServer("127.0.0.1:0", chain.Repo(), chain.Stater(), pool, chain.LogDB(),
		chain.Engine(), noPeers{}, chain.GetForkConfig(), cfg)
	require.NoError(t, err)
	t.Cleanup(closeSrv)
	return strings.TrimSuffix(strings.TrimPrefix(url, "http://"), "/")
}

// TestAPIServerWriteDeadlineCoversUnroutedReplies makes sure that the server
// closes the connection to a client that does not read, also for the 404, 405,
// and preflight replies.
func TestAPIServerWriteDeadlineCoversUnroutedReplies(t *testing.T) {
	const writeTimeout = 300 * time.Millisecond
	addr := startTestAPIServer(t, APIConfig{WriteTimeout: writeTimeout})

	const n = 20_000
	for name, req := range map[string]string{
		"matched route":  "GET /blocks/best HTTP/1.1\r\nHost: test\r\n\r\n",
		"404 not found":  "GET /no-such-route HTTP/1.1\r\nHost: test\r\n\r\n",
		"405 wrong verb": "POST /blocks/best HTTP/1.1\r\nHost: test\r\nContent-Length: 0\r\n\r\n",
		"CORS preflight": "OPTIONS /blocks/best HTTP/1.1\r\nHost: test\r\nOrigin: http://a.test\r\nAccess-Control-Request-Method: GET\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := pipelineAndStall(t, addr, req, n, 2*time.Second)
			assert.Less(t, got, n, "a stalled write outlived the %s write deadline", writeTimeout)
		})
	}
}

// TestAPIServerOversizedBodyGetsResponse is an end-to-end smoke check that an
// oversized request body through the wrapped conn still gets a complete reply. The
// status is 400, not 413: the handler maps the body limit error to Bad Request.
func TestAPIServerOversizedBodyGetsResponse(t *testing.T) {
	addr := startTestAPIServer(t, APIConfig{})

	resp, err := http.Post("http://"+addr+"/accounts/*", "application/json",
		bytes.NewReader(bytes.Repeat([]byte("x"), 300*1024)))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
