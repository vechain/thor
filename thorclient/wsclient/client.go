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

	"github.com/vechain/thor/v2/thor"

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
func (c *Client) SubscribeEvents(pos string, filter *subscriptions.EventFilter) (*common.Subscription[*subscriptions.EventMessage], error) {
	queryValues := &url.Values{}
	queryValues.Add("pos", pos)
	if filter != nil {
		if filter.Address != nil {
			queryValues.Add("address", filter.Address.String())
		}
		if filter.Topic0 != nil {
			queryValues.Add("topic0", filter.Topic0.String())
		}
		if filter.Topic1 != nil {
			queryValues.Add("topic1", filter.Topic1.String())
		}
		if filter.Topic2 != nil {
			queryValues.Add("topic2", filter.Topic2.String())
		}
		if filter.Topic3 != nil {
			queryValues.Add("topic3", filter.Topic3.String())
		}
		if filter.Topic4 != nil {
			queryValues.Add("topic4", filter.Topic4.String())
		}
	}
	conn, err := c.connect("/subscriptions/event", queryValues)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.EventMessage](conn), nil
}

// SubscribeBlocks subscribes to block updates based on the provided query.
// It returns a Subscription that streams block messages or an error if the connection fails.
func (c *Client) SubscribeBlocks(pos string) (*common.Subscription[*blocks.JSONCollapsedBlock], error) {
	queryValues := &url.Values{}
	queryValues.Add("pos", pos)
	conn, err := c.connect("/subscriptions/block", queryValues)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[blocks.JSONCollapsedBlock](conn), nil
}

// SubscribeTransfers subscribes to transfer events based on the provided query.
// It returns a Subscription that streams transfer messages or an error if the connection fails.
func (c *Client) SubscribeTransfers(pos string, filter *subscriptions.TransferFilter) (*common.Subscription[*subscriptions.TransferMessage], error) {
	queryValues := &url.Values{}
	queryValues.Add("pos", pos)
	if filter != nil {
		if filter.TxOrigin != nil {
			queryValues.Add("txOrigin", filter.TxOrigin.String())
		}
		if filter.Sender != nil {
			queryValues.Add("sender", filter.Sender.String())
		}
		if filter.Recipient != nil {
			queryValues.Add("recipient", filter.Recipient.String())
		}
	}
	conn, err := c.connect("/subscriptions/transfer", queryValues)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.TransferMessage](conn), nil
}

// SubscribeTxPool subscribes to pending transaction pool updates based on the provided query.
// It returns a Subscription that streams pending transaction messages or an error if the connection fails.
func (c *Client) SubscribeTxPool(txID *thor.Bytes32) (*common.Subscription[*subscriptions.PendingTxIDMessage], error) {
	queryValues := &url.Values{}
	if txID != nil {
		queryValues.Add("id", txID.String())
	}

	conn, err := c.connect("/subscriptions/txpool", queryValues)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	return subscribe[subscriptions.PendingTxIDMessage](conn), nil
}

// SubscribeBeats2 subscribes to Beat2 messages based on the provided query.
// It returns a Subscription that streams Beat2 messages or an error if the connection fails.
func (c *Client) SubscribeBeats2(pos string) (*common.Subscription[*subscriptions.Beat2Message], error) {
	queryValues := &url.Values{}
	queryValues.Add("pos", pos)
	conn, err := c.connect("/subscriptions/beat2", queryValues)
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
func (c *Client) connect(endpoint string, queryValues *url.Values) (*websocket.Conn, error) {
	u := url.URL{
		Scheme:   c.scheme,
		Host:     c.host,
		Path:     endpoint,
		RawQuery: queryValues.Encode(),
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
