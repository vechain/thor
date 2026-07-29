// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ws_test

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	rpcblocks "github.com/vechain/thor/v2/rpc/blocks"
	rpcchain "github.com/vechain/thor/v2/rpc/chain"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	rpcws "github.com/vechain/thor/v2/rpc/ws"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// fakeSyncer is a test double for rpcws.Syncer. Synced() returns a channel that
// the test can close via markSynced(); HighestPeerBlock() is settable.
type fakeSyncer struct {
	syncedCh chan struct{}
	highest  atomic.Uint32
}

func newFakeSyncer() *fakeSyncer { return &fakeSyncer{syncedCh: make(chan struct{})} }

func (f *fakeSyncer) Synced() <-chan struct{}  { return f.syncedCh }
func (f *fakeSyncer) HighestPeerBlock() uint32 { return f.highest.Load() }
func (f *fakeSyncer) markSynced() {
	select {
	case <-f.syncedCh:
	default:
		close(f.syncedCh)
	}
}
func (f *fakeSyncer) setHighest(n uint32) { f.highest.Store(n) }

type fixture struct {
	chain   *testchain.Chain
	pool    *txpool.TxPool
	srv     *httptest.Server
	handler *rpcws.Handler
	syncer  *fakeSyncer
}

func newFixture(t *testing.T) *fixture {
	return newFixtureWithSyncer(t, newFakeSyncer())
}

func newFixtureWithSyncer(t *testing.T, syncer *fakeSyncer) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	pool := txpool.New(c.Repo(), c.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &testchain.DefaultForkConfig)
	t.Cleanup(pool.Close)

	rpcSrv := jsonrpc.NewServer()
	rpcchain.New(c.Repo(), "test/1.0", syncer).Mount(rpcSrv)
	rpcblocks.New(c.Repo()).Mount(rpcSrv)

	h := rpcws.New(c.Repo(), pool, []string{"*"}, rpcSrv, syncer)
	t.Cleanup(h.Close)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &fixture{chain: c, pool: pool, srv: srv, handler: h, syncer: syncer}
}

// wsURL converts the test server's http:// URL to ws://.
func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/rpc"
}

// dial opens a WebSocket connection to the test server.
func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(srv.URL, "http://"), Path: "/rpc"}
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// rpcCall sends a JSON-RPC request over WS and reads the response.
func rpcCall(t *testing.T, conn *websocket.Conn, id int, method string, params any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, body))

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var resp struct {
		Result json.RawMessage   `json:"result"`
		Error  *jsonrpc.RPCError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(msg, &resp))
	require.Nil(t, resp.Error, "unexpected RPC error: %v", resp.Error)
	return resp.Result
}

// readNotification reads the next eth_subscription notification from the connection.
func readNotification(t *testing.T, conn *websocket.Conn, timeout time.Duration) (subID string, result json.RawMessage) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var notif struct {
		Method string `json:"method"`
		Params struct {
			Subscription string          `json:"subscription"`
			Result       json.RawMessage `json:"result"`
		} `json:"params"`
	}
	require.NoError(t, json.Unmarshal(msg, &notif))
	require.Equal(t, "eth_subscription", notif.Method)
	return notif.Params.Subscription, notif.Params.Result
}

// TestHTTPPassthrough verifies that plain HTTP POST requests still work after
// wrapping jsonrpc.Server with the WebSocket handler.
func TestHTTPPassthrough(t *testing.T) {
	fx := newFixture(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}`
	resp, err := http.Post(fx.srv.URL+"/rpc", "application/json", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result json.RawMessage   `json:"result"`
		Error  *jsonrpc.RPCError `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.NotEmpty(t, rpcResp.Result)
}

// TestNonSubscribeOverWS verifies that regular methods (eth_blockNumber) work
// over a WebSocket connection alongside subscriptions.
func TestNonSubscribeOverWS(t *testing.T) {
	fx := newFixture(t)
	conn := dial(t, fx.srv)

	result := rpcCall(t, conn, 1, "eth_blockNumber", []any{})
	var blockNum string
	require.NoError(t, json.Unmarshal(result, &blockNum))
	assert.Equal(t, "0x0", blockNum)
}

// TestBatchOverWS verifies that batch JSON-RPC requests work over WebSocket.
func TestBatchOverWS(t *testing.T) {
	fx := newFixture(t)
	conn := dial(t, fx.srv)

	batch := `[
		{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]},
		{"jsonrpc":"2.0","id":2,"method":"eth_chainId","params":[]}
	]`
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(batch)))

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var responses []struct {
		ID     json.RawMessage   `json:"id"`
		Result json.RawMessage   `json:"result"`
		Error  *jsonrpc.RPCError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(msg, &responses))
	require.Len(t, responses, 2)
	for _, r := range responses {
		assert.Nil(t, r.Error)
	}
}

// TestUnsubscribe verifies that eth_unsubscribe stops notifications.
func TestUnsubscribe(t *testing.T) {
	fx := newFixture(t)
	conn := dial(t, fx.srv)

	// Subscribe to newHeads.
	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"newHeads"})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))
	assert.Regexp(t, `^0x[0-9a-f]+$`, subID)

	// Unsubscribe.
	unsubResult := rpcCall(t, conn, 2, "eth_unsubscribe", []any{subID})
	var ok bool
	require.NoError(t, json.Unmarshal(unsubResult, &ok))
	assert.True(t, ok)

	// Unsubscribing again returns false.
	unsubResult2 := rpcCall(t, conn, 3, "eth_unsubscribe", []any{subID})
	var ok2 bool
	require.NoError(t, json.Unmarshal(unsubResult2, &ok2))
	assert.False(t, ok2)
}

// TestNewHeadsSubscription verifies that a newHeads subscription delivers a
// notification containing the new block's hash after a block is minted.
func TestNewHeadsSubscription(t *testing.T) {
	fx := newFixture(t)
	conn := dial(t, fx.srv)

	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"newHeads"})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	require.NoError(t, fx.chain.MintBlock())

	gotSubID, result := readNotification(t, conn, 3*time.Second)
	assert.Equal(t, subID, gotSubID)

	var block struct {
		Number string `json:"number"`
		Hash   string `json:"hash"`
	}
	require.NoError(t, json.Unmarshal(result, &block))
	assert.Equal(t, "0x1", block.Number)
	assert.Regexp(t, `^0x[0-9a-f]{64}$`, block.Hash)
}

// TestLogsSubscriptionNoEvents verifies that a plain ETH transfer (no events)
// does not trigger a notification on a logs subscription.
func TestLogsSubscriptionNoEvents(t *testing.T) {
	fx := newFixture(t)
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	conn := dial(t, fx.srv)

	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"logs", map[string]any{}})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	chainID := fx.chain.Repo().ChainID()
	ethTx := buildEthTx(t, chainID, sender, 0, &recipient.Address)
	require.NoError(t, fx.chain.MintBlock(ethTx))

	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "expected read timeout — plain transfer emits no logs")
}

// TestLogsSubscriptionWithEvents verifies that a logs subscription delivers a
// notification when an ETH-typed transaction emits a matching event.
// Uses Energy.transfer() which emits a Transfer(address,address,uint256) event.
func TestLogsSubscriptionWithEvents(t *testing.T) {
	fx := newFixture(t)
	sender := genesis.DevAccounts()[3]

	conn := dial(t, fx.srv)

	energyAddr := builtin.Energy.Address
	transferEvent, ok := builtin.Energy.ABI.EventByName("Transfer")
	require.True(t, ok)
	transferTopic := common.Hash(transferEvent.ID())

	// Subscribe filtering specifically for the Energy contract address.
	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"logs", map[string]any{
		"address": energyAddr.String(),
	}})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	// Build and mint a block containing an Energy.transfer call.
	recipient := genesis.DevAccounts()[1]
	transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)
	callData, err := transferMethod.EncodeInput(recipient.Address, big.NewInt(1e9))
	require.NoError(t, err)
	chainID := fx.chain.Repo().ChainID()
	ethCallTx := buildEthCallTx(t, chainID, sender, 0, &energyAddr, callData, 200_000)
	require.NoError(t, fx.chain.MintBlock(ethCallTx))

	// Expect a notification carrying the Transfer event log.
	gotSubID, result := readNotification(t, conn, 3*time.Second)
	assert.Equal(t, subID, gotSubID)

	var log struct {
		Address string   `json:"address"`
		Topics  []string `json:"topics"`
		Removed bool     `json:"removed"`
	}
	require.NoError(t, json.Unmarshal(result, &log))
	assert.True(t, strings.EqualFold(energyAddr.String(), log.Address))
	require.NotEmpty(t, log.Topics)
	assert.True(t, strings.EqualFold(transferTopic.Hex(), log.Topics[0]))
	assert.False(t, log.Removed)
}

// TestNewPendingTransactionsSubscription verifies that a newPendingTransactions
// subscription delivers the hash of an ETH-typed tx when it enters the pool.
func TestNewPendingTransactionsSubscription(t *testing.T) {
	fx := newFixture(t)
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	conn := dial(t, fx.srv)

	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"newPendingTransactions"})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	chainID := fx.chain.Repo().ChainID()
	ethTx := buildEthTx(t, chainID, sender, 0, &recipient.Address)
	require.NoError(t, fx.pool.Add(ethTx))

	gotSubID, result := readNotification(t, conn, 3*time.Second)
	assert.Equal(t, subID, gotSubID)

	var txHash string
	require.NoError(t, json.Unmarshal(result, &txHash))
	assert.Equal(t, "0x"+strings.ToLower(ethTx.ID().String()[2:]), strings.ToLower(txHash))
}

// TestUnsupportedSubscriptionType verifies that an unknown subscription type
// returns a JSON-RPC error.
func TestUnsupportedSubscriptionType(t *testing.T) {
	fx := newFixture(t)
	conn := dial(t, fx.srv)

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_subscribe",
		"params":  []any{"newBlocks"}, // not a recognised subscription type
	})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, body))

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var resp struct {
		Error *jsonrpc.RPCError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(msg, &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, jsonrpc.CodeInvalidParams, resp.Error.Code)
}

// TestSyncingSubscriptionAlreadySynced verifies that subscribing to "syncing"
// on a node that has already finished synchronising returns a single `false`
// notification and emits nothing further. Mirrors go-ethereum behaviour:
// when DownloaderAPI sees a new subscriber after sync is done, it pushes one
// `false` and goes quiet.
func TestSyncingSubscriptionAlreadySynced(t *testing.T) {
	syncer := newFakeSyncer()
	syncer.markSynced()
	fx := newFixtureWithSyncer(t, syncer)

	conn := dial(t, fx.srv)

	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"syncing"})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	gotSubID, result := readNotification(t, conn, 3*time.Second)
	assert.Equal(t, subID, gotSubID)

	// When the node is synced, the notification result is the JSON literal `false`.
	var done bool
	require.NoError(t, json.Unmarshal(result, &done))
	assert.False(t, done)

	// No further notifications expected.
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "expected read timeout — no further syncing notifications after the initial false")
}

// TestSyncingSubscriptionInProgress verifies that subscribing while the node
// is still syncing (Synced() not yet closed) immediately receives a progress
// notification with the three core fields, and that highestBlock is the max
// of the local head and the highest peer-advertised block.
func TestSyncingSubscriptionInProgress(t *testing.T) {
	syncer := newFakeSyncer()
	syncer.setHighest(1234) // pretend a peer is at block 1234
	fx := newFixtureWithSyncer(t, syncer)

	conn := dial(t, fx.srv)

	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"syncing"})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	gotSubID, result := readNotification(t, conn, 3*time.Second)
	assert.Equal(t, subID, gotSubID)

	var progress struct {
		Syncing bool `json:"syncing"`
		Status  struct {
			StartingBlock string `json:"startingBlock"`
			CurrentBlock  string `json:"currentBlock"`
			HighestBlock  string `json:"highestBlock"`
		} `json:"status"`
	}
	require.NoError(t, json.Unmarshal(result, &progress))
	assert.True(t, progress.Syncing)
	assert.Equal(t, "0x0", progress.Status.StartingBlock) // genesis-only chain
	assert.Equal(t, "0x0", progress.Status.CurrentBlock)
	assert.Equal(t, "0x4d2", progress.Status.HighestBlock) // 1234 hex
}

// TestSyncingSubscriptionCompletion verifies that when sync completes during
// an active subscription, the client receives a final `false` notification.
func TestSyncingSubscriptionCompletion(t *testing.T) {
	syncer := newFakeSyncer()
	syncer.setHighest(10)
	fx := newFixtureWithSyncer(t, syncer)

	conn := dial(t, fx.srv)

	subResult := rpcCall(t, conn, 1, "eth_subscribe", []any{"syncing"})
	var subID string
	require.NoError(t, json.Unmarshal(subResult, &subID))

	// Drain the initial in-progress notification.
	_, _ = readNotification(t, conn, 3*time.Second)

	// Sync completes server-side.
	syncer.markSynced()

	gotSubID, result := readNotification(t, conn, 3*time.Second)
	assert.Equal(t, subID, gotSubID)
	var done bool
	require.NoError(t, json.Unmarshal(result, &done))
	assert.False(t, done)
}

// TestWSURL is a quick smoke test confirming the wsURL helper produces the right scheme.
func TestWSURL(t *testing.T) {
	assert.Equal(t, "ws://example.com/rpc", wsURL(&httptest.Server{URL: "http://example.com"}))
}

// buildEthTx creates a minimal signed EIP-1559 transaction for testing.
func buildEthTx(t *testing.T, chainID uint64, sender genesis.DevAccount, nonce uint64, to *thor.Address) *tx.Transaction {
	t.Helper()
	unsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		Nonce(nonce).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		Gas(21000).
		To(to).
		Value(big.NewInt(1e9)).
		Build()
	ethTx, err := tx.Sign(unsigned, sender.PrivateKey)
	require.NoError(t, err)
	return ethTx
}

// buildEthCallTx creates a signed EIP-1559 contract call transaction for testing.
func buildEthCallTx(t *testing.T, chainID uint64, sender genesis.DevAccount, nonce uint64, to *thor.Address, data []byte, gas uint64) *tx.Transaction {
	t.Helper()
	unsigned := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		Nonce(nonce).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxFeePerGas(big.NewInt(2 * thor.InitialBaseFee)).
		Gas(gas).
		To(to).
		Data(data).
		Build()
	ethTx, err := tx.Sign(unsigned, sender.PrivateKey)
	require.NoError(t, err)
	return ethTx
}
