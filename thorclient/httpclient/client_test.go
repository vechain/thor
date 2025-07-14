// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpclient

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
)

func TestClient_GetTransactionReceipt(t *testing.T) {
	txID := thor.Bytes32{0x01}
	expectedReceipt := &api.Receipt{
		GasUsed:  1000,
		GasPayer: thor.Address{0x01},
		Paid:     &math.HexOrDecimal256{},
		Reward:   &math.HexOrDecimal256{},
		Reverted: false,
		Meta:     api.ReceiptMeta{},
		Outputs:  []*api.Output{},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/transactions/"+txID.String()+"/receipt", r.URL.Path)

		receiptBytes, _ := json.Marshal(expectedReceipt)
		w.Write(receiptBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	receipt, err := client.GetTransactionReceipt(&txID, "")

	assert.NoError(t, err)
	assert.Equal(t, expectedReceipt, receipt)
}

func TestClient_InspectClauses(t *testing.T) {
	calldata := &api.BatchCallData{}
	expectedResults := []*api.CallResult{{
		Data:      "data",
		Events:    []*api.Event{},
		Transfers: []*api.Transfer{},
		GasUsed:   1000,
		Reverted:  false,
		VMError:   "no error"}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts/*", r.URL.Path)

		inspectionResBytes, _ := json.Marshal(expectedResults)
		w.Write(inspectionResBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	results, err := client.InspectClauses(calldata, "")

	assert.NoError(t, err)
	assert.Equal(t, expectedResults, results)
}

func TestClient_SendTransaction(t *testing.T) {
	rawTx := &api.RawTx{}
	expectedResult := &api.SendTxResult{ID: &thor.Bytes32{0x01}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/transactions", r.URL.Path)

		txIDBytes, _ := json.Marshal(expectedResult)
		w.Write(txIDBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	result, err := client.SendTransaction(rawTx)

	assert.NoError(t, err)
	assert.Equal(t, expectedResult, result)
}

func TestClient_FilterTransfers(t *testing.T) {
	req := &api.TransferFilter{}
	expectedTransfers := []*api.FilteredTransfer{{
		Sender:    thor.Address{0x01},
		Recipient: thor.Address{0x02},
		Amount:    &math.HexOrDecimal256{},
		Meta:      api.LogMeta{},
	}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs/transfer", r.URL.Path)

		filteredTransfersBytes, _ := json.Marshal(expectedTransfers)
		w.Write(filteredTransfersBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	transfers, err := client.FilterTransfers(req)

	assert.NoError(t, err)
	assert.Equal(t, expectedTransfers, transfers)
}

func TestClient_FilterEvents(t *testing.T) {
	req := &api.EventFilter{}
	expectedEvents := []api.FilteredEvent{{
		Address: thor.Address{0x01},
		Topics:  []*thor.Bytes32{{0x01}},
		Data:    "data",
		Meta:    api.LogMeta{},
	}}
	expectedPath := "/logs/event"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, expectedPath, r.URL.Path)

		filteredEventsBytes, _ := json.Marshal(expectedEvents)
		w.Write(filteredEventsBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	events, err := client.FilterEvents(req)

	assert.NoError(t, err)
	assert.Equal(t, expectedEvents, events)
}

func TestClient_GetAccount(t *testing.T) {
	addr := thor.Address{0x01}
	expectedAccount := &api.Account{
		Balance: &math.HexOrDecimal256{},
		Energy:  &math.HexOrDecimal256{},
		HasCode: false,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts/"+addr.String(), r.URL.Path)

		accountBytes, _ := json.Marshal(expectedAccount)
		w.Write(accountBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	account, err := client.GetAccount(&addr, "")

	assert.NoError(t, err)
	assert.Equal(t, expectedAccount, account)
}

func TestClient_GetAccountCode(t *testing.T) {
	addr := thor.Address{0x01}
	expectedCodeRsp := &api.GetCodeResult{Code: hexutil.Encode([]byte{0x01, 0x03})}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts/"+addr.String()+"/code", r.URL.Path)

		marshal, err := json.Marshal(expectedCodeRsp)
		require.NoError(t, err)

		w.Write(marshal)
	}))
	defer ts.Close()

	client := New(ts.URL)
	byteCode, err := client.GetAccountCode(&addr, "")

	assert.NoError(t, err)
	assert.Equal(t, expectedCodeRsp.Code, byteCode.Code)
}

func TestClient_GetStorage(t *testing.T) {
	addr := thor.Address{0x01}
	key := thor.Bytes32{0x01}
	expectedStorageRsp := &api.GetStorageResult{Value: hexutil.Encode([]byte{0x01, 0x03})}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts/"+addr.String()+"/storage/"+key.String(), r.URL.Path)

		marshal, err := json.Marshal(expectedStorageRsp)
		require.NoError(t, err)

		w.Write(marshal)
	}))
	defer ts.Close()

	client := New(ts.URL)
	data, err := client.GetAccountStorage(&addr, &key, BestRevision)

	assert.NoError(t, err)
	assert.Equal(t, expectedStorageRsp.Value, data.Value)
}

func TestClient_GetExpandedBlock(t *testing.T) {
	blockID := "123"
	expectedBlock := &api.JSONExpandedBlock{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/blocks/"+blockID+"?expanded=true", r.URL.Path+"?"+r.URL.RawQuery)

		blockBytes, _ := json.Marshal(expectedBlock)
		w.Write(blockBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	block, err := client.GetExpandedBlock(blockID)

	assert.NoError(t, err)
	assert.Equal(t, expectedBlock, block)
}

func TestClient_GetBlock(t *testing.T) {
	blockID := "123"
	expectedBlock := &api.JSONCollapsedBlock{
		JSONBlockSummary: &api.JSONBlockSummary{
			Number:      123456,
			ID:          thor.Bytes32{0x01},
			GasLimit:    1000,
			Beneficiary: thor.Address{0x01},
			GasUsed:     100,
			TxsRoot:     thor.Bytes32{0x03},
			TxsFeatures: 1,
			IsFinalized: false,
		},
		Transactions: nil,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/blocks/"+blockID, r.URL.Path)

		blockBytes, _ := json.Marshal(expectedBlock)
		w.Write(blockBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	block, err := client.GetBlock(blockID)

	assert.NoError(t, err)
	assert.Equal(t, expectedBlock, block)
}

func TestClient_GetNilBlock(t *testing.T) {
	blockID := "123"
	var expectedBlock *api.JSONCollapsedBlock

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/blocks/"+blockID, r.URL.Path)

		w.Write([]byte("null"))
	}))
	defer ts.Close()

	client := New(ts.URL)
	block, err := client.GetBlock(blockID)

	assert.Equal(t, ErrNotFound, err)
	assert.Equal(t, expectedBlock, block)
}

func TestClient_GetTransaction(t *testing.T) {
	txID := thor.Bytes32{0x01}
	expectedTx := &transactions.Transaction{ID: txID}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/transactions/"+txID.String(), r.URL.Path)

		txBytes, _ := json.Marshal(expectedTx)
		w.Write(txBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	tx, err := client.GetTransaction(&txID, BestRevision, false)

	assert.NoError(t, err)
	assert.Equal(t, expectedTx, tx)
}

func TestClient_GetRawTransaction(t *testing.T) {
	txID := thor.Bytes32{0x01}
	expectedTx := &api.RawTransaction{
		Meta: &api.TxMeta{
			BlockID:        thor.Bytes32{0x01},
			BlockNumber:    1,
			BlockTimestamp: 123,
		},
		RawTx: api.RawTx{Raw: hexutil.Encode([]byte{0x03})},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/transactions/"+txID.String(), r.URL.Path)

		txBytes, err := json.Marshal(expectedTx)
		require.NoError(t, err)

		w.Write(txBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	tx, err := client.GetRawTransaction(&txID, BestRevision, false)

	assert.NoError(t, err)
	assert.Equal(t, expectedTx, tx)
}

func TestClient_GetFeesHistory(t *testing.T) {
	blockCount := uint32(5)
	newestBlock := "best"
	expectedFeesHistory := &api.FeesHistory{
		OldestBlock:   thor.Bytes32{0x01},
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(0x01))},
		GasUsedRatio:  []float64{0.0021},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/fees/history?blockCount="+fmt.Sprint(blockCount)+"&newestBlock="+newestBlock, r.URL.Path+"?"+r.URL.RawQuery)

		feesHistoryBytes, _ := json.Marshal(expectedFeesHistory)
		w.Write(feesHistoryBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	feesHistory, err := client.GetFeesHistory(blockCount, newestBlock, nil)

	assert.NoError(t, err)
	assert.Equal(t, expectedFeesHistory, feesHistory)
}

func TestClient_GetFeesHistoryWithRewardPercentiles(t *testing.T) {
	blockCount := uint32(5)
	newestBlock := "best"
	rewardPercentiles := []float64{10, 90}
	expectedFeesHistory := &api.FeesHistory{
		OldestBlock:   thor.Bytes32{0x01},
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(0x01))},
		GasUsedRatio:  []float64{0.0021},
		Reward: [][]*hexutil.Big{
			{
				(*hexutil.Big)(big.NewInt(0)),
				(*hexutil.Big)(big.NewInt(0)),
			},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rewardPercentilesStr := make([]string, len(rewardPercentiles))
		for i, p := range rewardPercentiles {
			rewardPercentilesStr[i] = fmt.Sprint(p)
		}
		assert.Equal(t, "/fees/history?blockCount="+fmt.Sprint(blockCount)+"&newestBlock="+newestBlock+"&rewardPercentiles="+strings.Join(rewardPercentilesStr, ","), r.URL.Path+"?"+r.URL.RawQuery)

		feesHistoryBytes, _ := json.Marshal(expectedFeesHistory)
		w.Write(feesHistoryBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	feesHistory, err := client.GetFeesHistory(blockCount, newestBlock, rewardPercentiles)

	assert.NoError(t, err)
	assert.Equal(t, expectedFeesHistory.OldestBlock, feesHistory.OldestBlock)
	assert.Equal(t, expectedFeesHistory.BaseFeePerGas, feesHistory.BaseFeePerGas)
	assert.Equal(t, expectedFeesHistory.GasUsedRatio, feesHistory.GasUsedRatio)
	for i, blockRewards := range feesHistory.Reward {
		for j, reward := range blockRewards {
			assert.Equal(t, expectedFeesHistory.Reward[i][j].String(), reward.String())
		}
	}
}

func TestClient_GetFeesPriority(t *testing.T) {
	expectedFeesPriority := &api.FeesPriority{
		MaxPriorityFeePerGas: (*hexutil.Big)(big.NewInt(0x20)),
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/fees/priority", r.URL.Path)

		feesPriorityBytes, _ := json.Marshal(expectedFeesPriority)
		w.Write(feesPriorityBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	feesPriority, err := client.GetFeesPriority()

	assert.NoError(t, err)
	assert.Equal(t, expectedFeesPriority, feesPriority)
}

func TestClient_RawHTTPPost(t *testing.T) {
	url := "/test"
	calldata := map[string]any{}
	expectedResponse := []byte{0x01}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, url, r.URL.Path)

		w.Write(expectedResponse)
	}))
	defer ts.Close()

	client := New(ts.URL)
	response, statusCode, err := client.RawHTTPPost(url, calldata)

	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, response)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestClient_RawHTTPGet(t *testing.T) {
	url := "/test"
	expectedResponse := []byte{0x01}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, url, r.URL.Path)

		w.Write(expectedResponse)
	}))
	defer ts.Close()

	client := New(ts.URL)
	response, statusCode, err := client.RawHTTPGet(url)

	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, response)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestClient_GetPeers(t *testing.T) {
	expectedPeers := []*api.PeerStats{{
		Name:        "nodeA",
		BestBlockID: thor.Bytes32{0x01},
		TotalScore:  1000,
		PeerID:      "peerId",
		NetAddr:     "netAddr",
		Inbound:     false,
		Duration:    1000,
	}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/node/network/peers", r.URL.Path)

		peersBytes, _ := json.Marshal(expectedPeers)
		w.Write(peersBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	peers, err := client.GetPeers()

	assert.NoError(t, err)
	assert.Equal(t, expectedPeers, peers)
}

func TestClient_Errors(t *testing.T) {
	txID := thor.Bytes32{0x01}
	blockID := "123"
	addr := thor.Address{0x01}

	for _, tc := range []struct {
		name     string
		path     string
		function any
	}{
		{
			name:     "TransactionReceipt",
			path:     "/transactions/" + txID.String() + "/receipt",
			function: func(client *Client) (*api.Receipt, error) { return client.GetTransactionReceipt(&txID, "") },
		},
		{
			name: "InspectClauses",
			path: "/accounts/*",
			function: func(client *Client) ([]*api.CallResult, error) {
				return client.InspectClauses(&api.BatchCallData{}, "")
			},
		},
		{
			name: "SendTransaction",
			path: "/transactions",
			function: func(client *Client) (*api.SendTxResult, error) {
				return client.SendTransaction(&api.RawTx{})
			},
		},
		{
			name: "FilterTransfers",
			path: "/logs/transfer",
			function: func(client *Client) ([]*api.FilteredTransfer, error) {
				return client.FilterTransfers(&api.TransferFilter{})
			},
		},
		{
			name: "FilterEvents",
			path: "/logs/event",
			function: func(client *Client) ([]api.FilteredEvent, error) {
				return client.FilterEvents(&api.EventFilter{})
			},
		},
		{
			name:     "Account",
			path:     "/accounts/" + addr.String(),
			function: func(client *Client) (*api.Account, error) { return client.GetAccount(&addr, "") },
		},
		{
			name:     "GetContractByteCode",
			path:     "/accounts/" + addr.String() + "/code",
			function: func(client *Client) (*api.GetCodeResult, error) { return client.GetAccountCode(&addr, "") },
		},
		{
			name: "GetAccountStorage",
			path: "/accounts/" + addr.String() + "/storage/" + thor.Bytes32{}.String(),
			function: func(client *Client) (*api.GetStorageResult, error) {
				return client.GetAccountStorage(&addr, &thor.Bytes32{}, BestRevision)
			},
		},
		{
			name:     "ExpandedBlock",
			path:     "/blocks/" + blockID + "?expanded=true",
			function: func(client *Client) (*api.JSONExpandedBlock, error) { return client.GetExpandedBlock(blockID) },
		},
		{
			name:     "Block",
			path:     "/blocks/" + blockID,
			function: func(client *Client) (*api.JSONCollapsedBlock, error) { return client.GetBlock(blockID) },
		},
		{
			name: "Transaction",
			path: "/transactions/" + txID.String(),
			function: func(client *Client) (*transactions.Transaction, error) {
				return client.GetTransaction(&txID, BestRevision, false)
			},
		},
		{
			name:     "Peers",
			path:     "/node/network/peers",
			function: func(client *Client) ([]*api.PeerStats, error) { return client.GetPeers() },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, tc.path, r.URL.Path)

				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer ts.Close()

			client := New(ts.URL)

			fn := reflect.ValueOf(tc.function)
			result := fn.Call([]reflect.Value{reflect.ValueOf(client)})

			if result[len(result)-1].IsNil() {
				t.Errorf("expected error for %s, but got nil", tc.name)
				return
			}

			err := result[len(result)-1].Interface().(error)
			assert.Error(t, err)
		})
	}
}
