// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ws

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/subscriptions"
)

func TestClient_SubscribeEvents(t *testing.T) {
	query := "exampleQuery"
	expectedEvent := &subscriptions.EventMessage{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/event", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		conn.WriteJSON(expectedEvent)
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	eventChan, err := client.SubscribeEvents(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedEvent, <-eventChan)
}

func TestClient_SubscribeBlocks(t *testing.T) {
	query := "exampleQuery"
	expectedBlock := &blocks.JSONBlockSummary{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/block", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		conn.WriteJSON(expectedBlock)
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	blockChan, err := client.SubscribeBlocks(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedBlock, <-blockChan)
}

func TestClient_SubscribeTransfers(t *testing.T) {
	query := "exampleQuery"
	expectedTransfer := &subscriptions.TransferMessage{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/transfer", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		conn.WriteJSON(expectedTransfer)
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	transferChan, err := client.SubscribeTransfers(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedTransfer, <-transferChan)
}

func TestClient_SubscribeBeats(t *testing.T) {
	query := "exampleQuery"
	expectedBeat := &subscriptions.BeatMessage{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/beats", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		conn.WriteJSON(expectedBeat)
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	beatChan, err := client.SubscribeBeats(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedBeat, <-beatChan)
}

func TestClient_SubscribeTxPool(t *testing.T) {
	query := "exampleQuery"
	expectedPendingTxID := &subscriptions.PendingTxIDMessage{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/txpool", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		conn.WriteJSON(expectedPendingTxID)
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	pendingTxIDChan, err := client.SubscribeTxPool(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedPendingTxID, <-pendingTxIDChan)
}

func TestClient_SubscribeBeats2(t *testing.T) {
	query := "exampleQuery"
	expectedBeat2 := &subscriptions.Beat2Message{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/beat2", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		conn.WriteJSON(expectedBeat2)
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	beat2Chan, err := client.SubscribeBeats2(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedBeat2, <-beat2Chan)
}
func TestNewClient(t *testing.T) {
	expectedHost := "example.com"

	for _, tc := range []struct {
		name           string
		url            string
		expectedSchema string
	}{
		{
			name:           "http",
			url:            "http://example.com",
			expectedSchema: "ws",
		},
		{
			name:           "https",
			url:            "https://example.com",
			expectedSchema: "wss",
		},
		{
			name:           "ws",
			url:            "ws://example.com",
			expectedSchema: "ws",
		},
		{
			name:           "wss",
			url:            "wss://example.com",
			expectedSchema: "wss",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(tc.url)
			assert.NoError(t, err)
			assert.NotNil(t, client)
			assert.Equal(t, tc.expectedSchema, client.scheme)
			assert.Equal(t, expectedHost, client.host)
		})
	}
}

func TestNewClientError(t *testing.T) {
	badURL := "invalid"
	client, err := NewClient(badURL)
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestClient_SubscribeError(t *testing.T) {
	query := "exampleQuery"
	badURL := "http://example.com"
	client, err := NewClient(badURL)
	assert.NoError(t, err)

	for _, tc := range []struct {
		name          string
		subscribeFunc interface{}
	}{
		{
			name:          "SubscribeEvents",
			subscribeFunc: client.SubscribeEvents,
		},
		{
			name:          "SubscribeTransfers",
			subscribeFunc: client.SubscribeTransfers,
		},
		{
			name:          "SubscribeBeats",
			subscribeFunc: client.SubscribeBeats,
		},
		{
			name:          "SubscribeTxPool",
			subscribeFunc: client.SubscribeTxPool,
		},
		{
			name:          "SubscribeBeats2",
			subscribeFunc: client.SubscribeBeats2,
		},
		{
			name:          "SubscribeBlocks",
			subscribeFunc: client.SubscribeBlocks,
		},
	} {
		fn := reflect.ValueOf(tc.subscribeFunc)
		result := fn.Call([]reflect.Value{reflect.ValueOf(query)})

		if result[1].IsNil() {
			t.Errorf("expected error for %s, but got nil", tc.name)
			return
		}

		err := result[1].Interface().(error)
		assert.Error(t, err)
	}
}
