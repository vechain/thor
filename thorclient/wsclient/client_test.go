// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package wsclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/thorclient/common"
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
	sub, err := client.SubscribeEvents(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedEvent, (<-sub.EventChan).Data)
}

func TestClient_SubscribeBlocks(t *testing.T) {
	query := "exampleQuery"
	expectedBlock := &blocks.JSONCollapsedBlock{}

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
	sub, err := client.SubscribeBlocks(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedBlock, (<-sub.EventChan).Data)
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
	sub, err := client.SubscribeTransfers(query)

	assert.NoError(t, err)
	derp := (<-sub.EventChan).Data
	assert.Equal(t, expectedTransfer, derp)
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
	sub, err := client.SubscribeTxPool(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedPendingTxID, (<-sub.EventChan).Data)
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
	sub, err := client.SubscribeBeats2(query)

	assert.NoError(t, err)
	assert.Equal(t, expectedBeat2, (<-sub.EventChan).Data)
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

func TestClient_SubscribeBlocks_ServerError(t *testing.T) {
	query := ""
	expectedError := "test error"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/block", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		// Send a message that causes an error on the client side
		conn.WriteMessage(websocket.TextMessage, []byte(expectedError))
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	sub, err := client.SubscribeBlocks(query)

	assert.NoError(t, err)

	// Read the error from the event channel
	event := <-sub.EventChan
	assert.Error(t, event.Error)
	assert.True(t, errors.Is(event.Error, common.ErrUnexpectedMsg))
}

func TestClient_SubscribeBlocks_ServerShutdown(t *testing.T) {
	query := "exampleQuery"
	expectedBlock := &blocks.JSONCollapsedBlock{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/block", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)

		// Send a valid block to the client
		conn.WriteJSON(expectedBlock)

		// Simulate a server shutdown by closing the WebSocket connection
		conn.Close()
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	sub, err := client.SubscribeBlocks(query)

	assert.NoError(t, err)

	// The first event should be the valid block
	event := <-sub.EventChan
	assert.NoError(t, event.Error)
	assert.Equal(t, expectedBlock, event.Data)

	// The next event should be an error due to the server shutdown
	event = <-sub.EventChan
	assert.Error(t, event.Error)
	assert.Contains(t, event.Error.Error(), "websocket: close")
}

func TestClient_SubscribeBlocks_ClientShutdown(t *testing.T) {
	query := "exampleQuery"
	expectedBlock := &blocks.JSONCollapsedBlock{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/block", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)

		// Send a valid block to the client

		for {
			err := conn.WriteJSON(expectedBlock)
			if err != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	sub, err := client.SubscribeBlocks(query)

	assert.NoError(t, err)

	// The first 50 events should be the valid block
	// the server is producing events at high speed
	for i := 0; i < 50; i++ {
		event := <-sub.EventChan
		assert.NoError(t, event.Error)
		assert.Equal(t, expectedBlock, event.Data)
	}

	// unsubscribe should close the connection forcing a connection error in the eventChan
	sub.Unsubscribe()

	// next message should be an error
	assert.Error(t, (<-sub.EventChan).Error)

	// Ensure no more events are received after unsubscribe
	select {
	case _, ok := <-sub.EventChan:
		if ok {
			t.Error("Expected the event channel to be closed after unsubscribe, but it was still open")
		}
	case <-time.After(200 * time.Second):
		// Timeout here is expected since the channel should be closed and not sending events
	}
}

func TestClient_SubscribeBlocks_ClientShutdown_LongBlocks(t *testing.T) {
	query := "exampleQuery"
	expectedBlock := &blocks.JSONCollapsedBlock{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/subscriptions/block", r.URL.Path)
		assert.Equal(t, query, r.URL.RawQuery)

		upgrader := websocket.Upgrader{}

		conn, _ := upgrader.Upgrade(w, r, nil)

		// Send a valid block to the client

		for {
			err := conn.WriteJSON(expectedBlock)
			if err != nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL)
	assert.NoError(t, err)
	sub, err := client.SubscribeBlocks(query)

	assert.NoError(t, err)

	assert.NoError(t, (<-sub.EventChan).Error)
	assert.NotNil(t, (<-sub.EventChan).Data)

	// unsubscribe should close the connection forcing a connection error in the eventChan
	sub.Unsubscribe()

	// Ensure no more events are received after unsubscribe
	select {
	case _, ok := <-sub.EventChan:
		if ok {
			t.Error("Expected the event channel to be closed after unsubscribe, but it was still open")
		}
	case <-time.After(200 * time.Millisecond):
		// Timeout here is expected since the channel should be closed and not sending events
	}
}

// go test -timeout 80s -run ^TestSubscribeBeats2WithServer$ github.com/vechain/thor/v2/thorclient/wsclient -v
func TestSubscribeBeats2WithServer(t *testing.T) {
	// t.Skip("this is a manual test")
	client, err := NewClient("https://mainnet.vechain.org")
	if err != nil {
		t.Fatal(err)
	}

	sub, err := client.SubscribeBeats2("")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		<-time.After(60 * time.Second)
		sub.Unsubscribe()
	}()

	for ev := range sub.EventChan {
		if ev.Error != nil {
			t.Fatal(ev.Error)
		}
		t.Log(ev.Data)
	}
}
