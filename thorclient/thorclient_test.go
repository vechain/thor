// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thorclient

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testnode"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	_ "github.com/vechain/thor/v2/tracers/native"
	"github.com/vechain/thor/v2/tx"
)

func TestConvertToBatchCallData(t *testing.T) {
	// Test case 1: Empty transaction
	tx1 := tx.NewBuilder(tx.TypeLegacy).Build()
	addr1 := &thor.Address{}
	expected1 := &api.BatchCallData{
		Clauses:    make(api.Clauses, 0),
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
		function         any
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
		function  any
		isPending bool
	}{
		{
			name:      "Transaction",
			function:  func(client *Client) { client.Transaction(&expectedTx.ID) },
			isPending: false,
		},
		{
			name:      "GetTransactionPending",
			function:  func(client *Client) { client.Transaction(&expectedTx.ID, Revision(httpclient.BestRevision), Pending()) },
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

func TestClient_DebugRevertedTransaction(t *testing.T) {
	node, err := testnode.NewNodeBuilder().Build()
	assert.NoError(t, err)
	require.NoError(t, node.Start())
	t.Cleanup(func() {
		require.NoError(t, node.Stop())
	})

	txThatReverts := tx.NewBuilder(tx.TypeLegacy).
		Gas(100_000).
		Expiration(1000).
		GasPriceCoef(255).
		ChainTag(node.Chain().ChainTag())

	method, ok := builtin.Params.ABI.MethodByName("set")
	assert.True(t, ok)

	input, err := method.EncodeInput(thor.Bytes32{}, big.NewInt(1))
	assert.NoError(t, err)
	clause := tx.NewClause(&builtin.Params.Address).WithData(input)

	txThatReverts.Clause(clause)

	trx := txThatReverts.Build()
	trx = tx.MustSign(trx, genesis.DevAccounts()[1].PrivateKey)
	require.NoError(t, node.Chain().MintBlock(genesis.DevAccounts()[0], trx))

	client := New(node.APIServer().URL)
	id := trx.ID()
	data, err := client.DebugRevertedTransaction(&id)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "builtin: executor required")
}
