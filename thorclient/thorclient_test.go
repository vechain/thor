package client

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestWs_Error(t *testing.T) {
	client := NewClient("http://test.com")

	for _, tc := range []struct {
		name     string
		function interface{}
	}{
		{
			name:     "SubscribeBlocks",
			function: client.SubscribeBlocks,
		},
		{
			name:     "SubscribeEvents",
			function: client.SubscribeEvents,
		},
		{
			name:     "SubscribeTransfers",
			function: client.SubscribeTransfers,
		},
		{
			name:     "SubscribeTxPool",
			function: client.SubscribeTxPool,
		},
		{
			name:     "SubscribeBeats",
			function: client.SubscribeBeats,
		},
		{
			name:     "SubscribeBeats2",
			function: client.SubscribeBeats2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := reflect.ValueOf(tc.function)
			result := fn.Call([]reflect.Value{})

			if result[1].IsNil() {
				t.Errorf("expected error for %s, but got nil", tc.name)
				return
			}

			err := result[1].Interface().(error)
			assert.Error(t, err)
		})
	}
}

func TestConvertToBatchCallData(t *testing.T) {
	// Test case 1: Empty transaction
	tx1 := &tx.Transaction{}
	addr1 := &thor.Address{}
	expected1 := &accounts.BatchCallData{
		Clauses:    make(accounts.Clauses, 0),
		Gas:        0,
		ProvedWork: nil,
		Caller:     addr1,
		GasPayer:   nil,
		Expiration: 0,
		BlockRef:   "0x0000000000000000",
	}
	assert.Equal(t, expected1, convertToBatchCallData(tx1, addr1))
}
