// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package wsclient

// EventWrapper is used to return errors from the websocket alongside the data
type EventWrapper[T any] struct {
	Data  T
	Error error
}

// Subscription is used to handle the active subscription
type Subscription[T any] struct {
	EventChan   <-chan EventWrapper[T]
	Unsubscribe func() error
}
