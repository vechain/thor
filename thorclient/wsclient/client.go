// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package wsclient provides a WebSocket client for subscribing to various VeChainThor blockchain events.
// It enables subscriptions to blocks, transfers, events, and other updates via WebSocket.
package wsclient

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/thorclient/common"
)

const readTimeout = 60 * time.Second

// Client represents a WebSocket client that connects to the VeChainThor blockchain via WebSocket
// for subscribing to blockchain events and updates.
type Client struct {
	host   string
	scheme string
}

// NewClient creates a new WebSocket Client from the provided URL.
// The function parses the URL, determines the appropriate WebSocket scheme (ws or wss),
// and returns the client or an error if the URL is invalid.
func NewClient(url string) (*Client, error) {
	var host string
	var scheme string

	// Determine the scheme (ws or wss) based on the URL.
	if strings.Contains(url, "https://") || strings.Contains(url, "wss://") {
		host = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "wss://")
		scheme = "wss"
	} else if strings.Contains(url, "http://") || strings.Contains(url, "ws://") {
		host = strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "ws://")
		scheme = "ws"
	} else {
		return nil, fmt.Errorf("invalid url")
	}

	return &Client{
		host:   strings.TrimSuffix(host, "/"),
		scheme: scheme,
	}, nil
}

// SubscribeEvents subscribes to blockchain events based on the provided query.
// It returns a Subscription that streams event messages or an error if the connection fails.
func (c *Client) SubscribeEvents(query string) (*common.Subscription[*subscriptions.EventMessage], error) {
	conn, err := c.connect("/subscriptions/event", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.EventMessage](conn), nil
}

// SubscribeBlocks subscribes to block updates based on the provided query.
// It returns a Subscription that streams block messages or an error if the connection fails.
func (c *Client) SubscribeBlocks(query string) (*common.Subscription[*blocks.JSONCollapsedBlock], error) {
	conn, err := c.connect("/subscriptions/block", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[blocks.JSONCollapsedBlock](conn), nil
}

// SubscribeTransfers subscribes to transfer events based on the provided query.
// It returns a Subscription that streams transfer messages or an error if the connection fails.
func (c *Client) SubscribeTransfers(query string) (*common.Subscription[*subscriptions.TransferMessage], error) {
	conn, err := c.connect("/subscriptions/transfer", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.TransferMessage](conn), nil
}

// SubscribeTxPool subscribes to pending transaction pool updates based on the provided query.
// It returns a Subscription that streams pending transaction messages or an error if the connection fails.
func (c *Client) SubscribeTxPool(query string) (*common.Subscription[*subscriptions.PendingTxIDMessage], error) {
	conn, err := c.connect("/subscriptions/txpool", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.PendingTxIDMessage](conn), nil
}

// SubscribeBeats2 subscribes to Beat2 messages based on the provided query.
// It returns a Subscription that streams Beat2 messages or an error if the connection fails.
func (c *Client) SubscribeBeats2(query string) (*common.Subscription[*subscriptions.Beat2Message], error) {
	conn, err := c.connect("/subscriptions/beat2", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.Beat2Message](conn), nil
}

// subscribe starts a new subscription over the given WebSocket connection.
// It returns a read-only channel that streams events of type T.
func subscribe[T any](conn *websocket.Conn) *common.Subscription[*T] {
	// Create a new channel for events
	eventChan := make(chan common.EventWrapper[*T], 1_000)
	var closed bool

	// Start a goroutine to handle receiving messages from the WebSocket connection.
	go func() {
		defer close(eventChan)
		defer conn.Close()

		for {
			conn.SetReadDeadline(time.Now().Add(readTimeout))
			var data T
			// Read a JSON message from the WebSocket and unmarshal it into the data.
			err := conn.ReadJSON(&data)
			if err != nil {
				if !closed {
					// Send an EventWrapper with the error to the channel.
					eventChan <- common.EventWrapper[*T]{Error: fmt.Errorf("%w: %w", common.ErrUnexpectedMsg, err)}
				}
				return
			}

			eventChan <- common.EventWrapper[*T]{Data: &data}
		}
	}()

	return &common.Subscription[*T]{
		EventChan: eventChan,
		Unsubscribe: func() {
			closed = true
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			conn.Close()
		},
	}
}

// connect establishes a WebSocket connection to the specified endpoint and query.
// It returns the connection or an error if the connection fails.
func (c *Client) connect(endpoint, rawQuery string) (*websocket.Conn, error) {
	u := url.URL{
		Scheme:   c.scheme,
		Host:     c.host,
		Path:     endpoint,
		RawQuery: rawQuery,
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}

	conn.SetPingHandler(func(payload string) error {
		// Make a best effort to send the pong message.
		_ = conn.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second))
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})
	// TODO append to the connection pool
	return conn, nil
}
