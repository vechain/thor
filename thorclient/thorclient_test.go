// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thorclient

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"

	tccommon "github.com/vechain/thor/v2/thorclient/common"
)

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

func TestRevision(t *testing.T) {
	addr := thor.BytesToAddress([]byte("account1"))
	revision := "revision1"

	for _, tc := range []struct {
		name             string
		function         interface{}
		expectedPath     string
		expectedRevision string
	}{
		{
			name:             "Account",
			function:         func(client *Client) { client.Account(&addr) },
			expectedPath:     "/accounts/" + addr.String(),
			expectedRevision: "",
		},
		{
			name:             "GetAccounForRevision",
			function:         func(client *Client) { client.Account(&addr, Revision(revision)) },
			expectedPath:     "/accounts/" + addr.String(),
			expectedRevision: "",
		},
		{
			name:             "GetAccountCode",
			function:         func(client *Client) { client.AccountCode(&addr) },
			expectedPath:     "/accounts/" + addr.String() + "/code",
			expectedRevision: "",
		},
		{
			name:             "GetAccountCodeForRevision",
			function:         func(client *Client) { client.AccountCode(&addr, Revision(revision)) },
			expectedPath:     "/accounts/" + addr.String() + "/code",
			expectedRevision: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.expectedPath, r.URL.Path)
				if tc.expectedRevision != "" {
					assert.Equal(t, "revision", r.URL.Query().Get("revision"))
				}

				w.Write([]byte{})
			}))
			defer ts.Close()

			client := New(ts.URL)

			fn := reflect.ValueOf(tc.function)
			fn.Call([]reflect.Value{reflect.ValueOf(client)})
		})
	}
}

func TestGetTransaction(t *testing.T) {
	expectedTx := &transactions.Transaction{
		ID: thor.BytesToBytes32([]byte("txid1")),
	}

	for _, tc := range []struct {
		name      string
		function  interface{}
		isPending bool
	}{
		{
			name:      "Transaction",
			function:  func(client *Client) { client.Transaction(&expectedTx.ID) },
			isPending: false,
		},
		{
			name:      "GetTransactionPending",
			function:  func(client *Client) { client.Transaction(&expectedTx.ID, Revision(tccommon.BestRevision), Pending()) },
			isPending: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/transactions/"+expectedTx.ID.String(), r.URL.Path)
				if tc.isPending {
					assert.Equal(t, "true", r.URL.Query().Get("pending"))
				}

				w.Write(expectedTx.ID[:])
			}))
			defer ts.Close()

			client := New(ts.URL)
			fn := reflect.ValueOf(tc.function)
			fn.Call([]reflect.Value{reflect.ValueOf(client)})
		})
	}
}
