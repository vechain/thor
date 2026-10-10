// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestEventFilter_Match(t *testing.T) {
	// Create an event filter
	addr := thor.BytesToAddress([]byte("address"))
	filter := &SubscriptionEventFilter{
		Address: &addr,
		Topic0:  &thor.Bytes32{0x01},
		Topic1:  &thor.Bytes32{0x02},
		Topic2:  &thor.Bytes32{0x03},
		Topic3:  &thor.Bytes32{0x04},
		Topic4:  &thor.Bytes32{0x05},
	}

	// Create an event that matches the filter
	event := &tx.Event{
		Address: addr,
		Topics: []thor.Bytes32{
			{0x01},
			{0x02},
			{0x03},
			{0x04},
			{0x05},
		},
	}
	assert.True(t, filter.Match(event))

	// Create an event that does not match the filter address
	event = &tx.Event{
		Address: thor.BytesToAddress([]byte("other_address")),
		Topics: []thor.Bytes32{
			{0x01},
			{0x02},
			{0x03},
			{0x04},
			{0x05},
		},
	}
	assert.False(t, filter.Match(event))

	// Create an event that does not match a filter topic
	event = &tx.Event{
		Address: addr,
		Topics: []thor.Bytes32{
			{0x05},
			{0x04},
			{0x03},
			{0x02},
			{0x01},
		},
	}
	assert.False(t, filter.Match(event))

	// Create an event that does not match a filter topic len
	event = &tx.Event{
		Address: addr,
		Topics:  []thor.Bytes32{{0x01}},
	}
	assert.False(t, filter.Match(event))
}

func TestTransferFilter_Match(t *testing.T) {
	// Create a transfer filter
	origin := thor.BytesToAddress([]byte("origin"))
	sender := thor.BytesToAddress([]byte("sender"))
	recipient := thor.BytesToAddress([]byte("recipient"))
	filter := &SubscriptionTransferFilter{
		TxOrigin:  &origin,
		Sender:    &sender,
		Recipient: &recipient,
	}

	// Create a transfer that matches the filter
	transfer := &tx.Transfer{
		Sender:    thor.BytesToAddress([]byte("sender")),
		Recipient: thor.BytesToAddress([]byte("recipient")),
		Amount:    big.NewInt(100),
	}
	assert.True(t, filter.Match(transfer, origin))

	// Create a transfer that does not match the filter
	transfer = &tx.Transfer{
		Sender:    thor.BytesToAddress([]byte("other_sender")),
		Recipient: thor.BytesToAddress([]byte("recipient")),
		Amount:    big.NewInt(100),
	}
	assert.False(t, filter.Match(transfer, origin))
	assert.False(t, filter.Match(transfer, thor.BytesToAddress(nil)))
	transfer = &tx.Transfer{
		Sender:    sender,
		Recipient: thor.BytesToAddress([]byte("other_recipient")),
		Amount:    big.NewInt(100),
	}
	assert.False(t, filter.Match(transfer, origin))
}
