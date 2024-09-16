// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/eventcontract"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var ts *httptest.Server
var blocks []*block.Block

func TestSubscriptions(t *testing.T) {
	initSubscriptionsServer(t)
	defer ts.Close()

	for name, tt := range map[string]func(*testing.T){
		"testHandleSubjectWithBlock":            testHandleSubjectWithBlock,
		"testHandleSubjectWithEvent":            testHandleSubjectWithEvent,
		"testHandleSubjectWithTransfer":         testHandleSubjectWithTransfer,
		"testHandleSubjectWithBeat":             testHandleSubjectWithBeat,
		"testHandleSubjectWithBeat2":            testHandleSubjectWithBeat2,
		"testHandleSubjectWithNonValidArgument": testHandleSubjectWithNonValidArgument,
	} {
		t.Run(name, tt)
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
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tr := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err := crypto.Sign(tr.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tr = tr.WithSignature(sig)

	txDeploy := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(100).
		Gas(1_000_000).
		Nonce(3).
		Clause(tx.NewClause(nil).WithData(common.Hex2Bytes(eventcontract.HexBytecode))).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	sigTxDeploy, err := crypto.Sign(txDeploy.SigningHash().Bytes(), genesis.DevAccounts()[1].PrivateKey)
	require.NoError(t, err)
	txDeploy = txDeploy.WithSignature(sigTxDeploy)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], tr, txDeploy))

	blocks, err = thorChain.GetAllBlocks()
	require.NoError(t, err)

	router := mux.NewRouter()
	New(thorChain.Repo(), []string{}, 5, txPool).
		Mount(router, "/subscriptions")
	ts = httptest.NewServer(router)
}

func TestSubscriptionsBacktrace(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tr := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err := crypto.Sign(tr.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tr = tr.WithSignature(sig)

	txDeploy := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(100).
		Gas(1_000_000).
		Nonce(3).
		Clause(tx.NewClause(nil).WithData(common.Hex2Bytes(eventcontract.HexBytecode))).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	sigTxDeploy, err := crypto.Sign(txDeploy.SigningHash().Bytes(), genesis.DevAccounts()[1].PrivateKey)
	require.NoError(t, err)
	txDeploy = txDeploy.WithSignature(sigTxDeploy)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], tr, txDeploy))

	for i := 0; i < 10; i++ {
		require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0]))
	}

	blocks, err = thorChain.GetAllBlocks()
	require.NoError(t, err)

	router := mux.NewRouter()
	New(thorChain.Repo(), []string{}, 5, txPool).Mount(router, "/subscriptions")
	ts = httptest.NewServer(router)

	defer ts.Close()

	t.Run("testHandleSubjectWithTransferBacktraceLimit", testHandleSubjectWithTransferBacktraceLimit)
}
func testHandleSubjectWithTransferBacktraceLimit(t *testing.T) {
	genesisBlock := blocks[0]
	queryArg := fmt.Sprintf("pos=%s", genesisBlock.Header().ID().String())
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(ts.URL, "http://"), Path: "/subscriptions/transfer", RawQuery: queryArg}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.Error(t, err)
	assert.Equal(t, "websocket: bad handshake", err.Error())
	defer resp.Body.Close() // Ensure body is closed after reading

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	assert.Equal(t, body, []byte("pos: backtrace limit exceeded\n"))
	assert.Nil(t, conn)
}
