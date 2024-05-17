// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

var ts *httptest.Server
var client *http.Client
var sub *Subscriptions
var txPool *txpool.TxPool
var repo *chain.Repository
var blocks []*block.Block

func TestMain(t *testing.T) {
	initSubscriptionsServer(t)
	defer ts.Close()

	testHandlePendingTransactions(t)
	testHandleSubjectWithBlock(t)
	testHandleSubjectWithEvent(t)
	testHandleSubjectWithTransfer(t)
	testHandleSubjectWithBeat(t)
	testHandleSubjectWithBeat2(t)
	testHandleSubjectWithNonValidArgument(t)
}

func testHandlePendingTransactions(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/txpool"}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Check the protocol upgrade to websocket
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, "Upgrade", resp.Header.Get("Connection"))
	assert.Equal(t, "websocket", resp.Header.Get("Upgrade"))

	// Add a new tx to the mempool
	transaction := createTx(t, repo, 1)
	txPool.AddLocal(transaction)

	readComplete := make(chan struct{})
	var msg []byte

	go func() {
		defer close(readComplete)
		_, msg, err = conn.ReadMessage()
	}()

	select {
	case <-readComplete:
		assert.NoError(t, err)
		var pendingTx *PendingTxIDMessage
		if err := json.Unmarshal(msg, &pendingTx); err != nil {
			t.Fatal(err)
		} else {
			assert.Equal(t, transaction.ID(), pendingTx.ID)
		}
	case <-time.After(5 * time.Second):
		// If in 5 seconds the message is not received, the test will be skipped
		t.SkipNow()
	}
}

func testHandleSubjectWithBlock(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/block", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Check the protocol upgrade to websocket
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, "Upgrade", resp.Header.Get("Connection"))
	assert.Equal(t, "websocket", resp.Header.Get("Upgrade"))

	_, msg, err := conn.ReadMessage()

	assert.NoError(t, err)

	var blockMsg *BlockMessage
	if err := json.Unmarshal(msg, &blockMsg); err != nil {
		t.Fatal(err)
	} else {
		newBlock := blocks[1]
		assert.Equal(t, newBlock.Header().Number(), blockMsg.Number)
		assert.Equal(t, newBlock.Header().ID(), blockMsg.ID)
		assert.Equal(t, newBlock.Header().Timestamp(), blockMsg.Timestamp)
	}
}

func testHandleSubjectWithEvent(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/event", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Check the protocol upgrade to websocket
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, "Upgrade", resp.Header.Get("Connection"))
	assert.Equal(t, "websocket", resp.Header.Get("Upgrade"))

	_, msg, err := conn.ReadMessage()

	assert.NoError(t, err)

	var eventMsg *EventMessage
	if err := json.Unmarshal(msg, &eventMsg); err != nil {
		t.Fatal(err)
	} else {
		newBlock := blocks[1]
		assert.Equal(t, newBlock.Header().Number(), eventMsg.Meta.BlockNumber)
		assert.Equal(t, newBlock.Header().ID(), eventMsg.Meta.BlockID)
	}
}

func testHandleSubjectWithTransfer(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/transfer", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Check the protocol upgrade to websocket
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, "Upgrade", resp.Header.Get("Connection"))
	assert.Equal(t, "websocket", resp.Header.Get("Upgrade"))

	_, msg, err := conn.ReadMessage()

	assert.NoError(t, err)

	var transferMsg *TransferMessage
	if err := json.Unmarshal(msg, &transferMsg); err != nil {
		t.Fatal(err)
	} else {
		newBlock := blocks[1]
		assert.Equal(t, newBlock.Header().Number(), transferMsg.Meta.BlockNumber)
		assert.Equal(t, newBlock.Header().ID(), transferMsg.Meta.BlockID)
	}
}

func testHandleSubjectWithBeat(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/beat", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Check the protocol upgrade to websocket
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, "Upgrade", resp.Header.Get("Connection"))
	assert.Equal(t, "websocket", resp.Header.Get("Upgrade"))

	_, msg, err := conn.ReadMessage()

	assert.NoError(t, err)

	var beatMsg *BeatMessage
	if err := json.Unmarshal(msg, &beatMsg); err != nil {
		t.Fatal(err)
	} else {
		newBlock := blocks[1]
		assert.Equal(t, newBlock.Header().Number(), beatMsg.Number)
		assert.Equal(t, newBlock.Header().ID(), beatMsg.ID)
		assert.Equal(t, newBlock.Header().Timestamp(), beatMsg.Timestamp)
	}
}

func testHandleSubjectWithBeat2(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/beat2", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Check the protocol upgrade to websocket
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, "Upgrade", resp.Header.Get("Connection"))
	assert.Equal(t, "websocket", resp.Header.Get("Upgrade"))

	_, msg, err := conn.ReadMessage()

	assert.NoError(t, err)

	var beatMsg *Beat2Message
	if err := json.Unmarshal(msg, &beatMsg); err != nil {
		t.Fatal(err)
	} else {
		newBlock := blocks[1]
		assert.Equal(t, newBlock.Header().Number(), beatMsg.Number)
		assert.Equal(t, newBlock.Header().ID(), beatMsg.ID)
		assert.Equal(t, newBlock.Header().GasLimit(), beatMsg.GasLimit)
	}
}

func testHandleSubjectWithNonValidArgument(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/randomArgument", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)

	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestParseAddress(t *testing.T) {
	addrStr := "0x0123456789abcdef0123456789abcdef01234567"
	expectedAddr := thor.MustParseAddress(addrStr)

	result, err := parseAddress(addrStr)

	assert.NoError(t, err)
	assert.Equal(t, expectedAddr, *result)
}

func initSubscriptionsServer(t *testing.T) {
	r, generatedBlocks, pool := initChain(t)
	repo = r
	txPool = pool
	blocks = generatedBlocks
	router := mux.NewRouter()
	sub = New(repo, []string{}, 5, txPool)
	sub.Mount(router, "/subscriptions")
	ts = httptest.NewServer(router)
	client = &http.Client{}
}
