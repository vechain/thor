// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package wsclient

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/thorclient/common"
)

type Client struct {
	host   string
	scheme string
}

func NewClient(url string) (*Client, error) {
	var host string
	var scheme string

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

func (c *Client) SubscribeEvents(query string) (*common.Subscription[*subscriptions.EventMessage], error) {
	conn, err := c.connect("/subscriptions/event", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	// ensure the reader is stopped before stopping the ws connection
	g := co.NewChoes()
	eventChan := subscribe[subscriptions.EventMessage](g, conn)

	return &common.Subscription[*subscriptions.EventMessage]{
		EventChan:   eventChan,
		Unsubscribe: stopFunc(g, eventChan),
	}, nil
}

func (c *Client) SubscribeBlocks(query string) (*common.Subscription[*blocks.JSONCollapsedBlock], error) {
	conn, err := c.connect("/subscriptions/block", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	// ensure the reader is stopped before stopping the ws connection
	g := co.NewChoes()
	eventChan := subscribe[blocks.JSONCollapsedBlock](g, conn)

	return &common.Subscription[*blocks.JSONCollapsedBlock]{
		EventChan:   eventChan,
		Unsubscribe: stopFunc(g, eventChan),
	}, nil
}

func (c *Client) SubscribeTransfers(query string) (*common.Subscription[*subscriptions.TransferMessage], error) {
	conn, err := c.connect("/subscriptions/transfer", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	// ensure the reader is stopped before stopping the ws connection
	g := co.NewChoes()
	eventChan := subscribe[subscriptions.TransferMessage](g, conn)

	return &common.Subscription[*subscriptions.TransferMessage]{
		EventChan:   eventChan,
		Unsubscribe: stopFunc(g, eventChan),
	}, nil
}

func (c *Client) SubscribeTxPool(query string) (*common.Subscription[*subscriptions.PendingTxIDMessage], error) {
	conn, err := c.connect("/subscriptions/txpool", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	// ensure the reader is stopped before stopping the ws connection
	g := co.NewChoes()
	eventChan := subscribe[subscriptions.PendingTxIDMessage](g, conn)

	return &common.Subscription[*subscriptions.PendingTxIDMessage]{
		EventChan:   eventChan,
		Unsubscribe: stopFunc(g, eventChan),
	}, nil
}

func (c *Client) SubscribeBeats2(query string) (*common.Subscription[*subscriptions.Beat2Message], error) {
	conn, err := c.connect("/subscriptions/beat2", query)
	if err != nil {
		return nil, fmt.Errorf("unable to connect - %w", err)
	}

	// ensure the reader is stopped before stopping the ws connection
	g := co.NewChoes()
	eventChan := subscribe[subscriptions.Beat2Message](g, conn)

	return &common.Subscription[*subscriptions.Beat2Message]{
		EventChan:   eventChan,
		Unsubscribe: stopFunc(g, eventChan),
	}, nil
}

// stopFunc ensure the reader is stopped before stopping the websocket connection
func stopFunc[T any](g *co.Choes, eventChan <-chan common.EventWrapper[T]) func() {
	return func() {
		g.Stop()

		// drain any pending messages
		go func() {
			for range eventChan {
				// consume messages until channel is closed
			}
		}()

		g.Wait()
	}
}

// subscribe creates a channel to handle new subscriptions
// It takes a websocket connection as an argument and returns a read-only channel for receiving messages of type T and an error if any occurs.
func subscribe[T any](g *co.Choes, conn *websocket.Conn) <-chan common.EventWrapper[*T] {
	// Create a new channel for events
	eventChan := make(chan common.EventWrapper[*T], 10_000)

	// Start a goroutine to handle receiving messages from the websocket connection
	// use the co.Choes that the client can wait for the stopChan signaling
	g.Go(func(stopChan chan struct{}) {
		defer close(eventChan)
		defer conn.Close()

		for {
			select {
			case <-stopChan:
				return
			default:
				var data T
				// Read a JSON message from the websocket and unmarshal it into data
				err := conn.ReadJSON(&data)
				if err != nil {
					// Send an EventWrapper with the error to the channel
					eventChan <- common.EventWrapper[*T]{Error: fmt.Errorf("%w: %w", common.ErrUnexpectedMsg, err)}
					return
				}

				// Send the received data to the event channel
				eventChan <- common.EventWrapper[*T]{Data: &data}
				// TODO: handle the case where data is invalid or undesirable
			}
		}
	})

	// Return the event channel
	return eventChan
}

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
	//TODO append to the connection pool
	return conn, nil
}
