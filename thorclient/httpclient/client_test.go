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

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/thor"
)

func assertHexOrDecimal256Equal(t *testing.T, expected, actual *math.HexOrDecimal256) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil || actual == nil {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
	expectedInt := (*big.Int)(expected)
	actualInt := (*big.Int)(actual)
	if expectedInt.Cmp(actualInt) != 0 {
		t.Fatalf("expected %v, got %v", expectedInt.String(), actualInt.String())
	}
}

func TestClient_GetTransactionReceipt(t *testing.T) {
	txID := thor.Bytes32{0x01}
	expectedReceipt := &dto.Receipt{
		GasUsed:  1000,
		GasPayer: thor.Address{0x01},
		Paid:     &math.HexOrDecimal256{},
		Reward:   &math.HexOrDecimal256{},
		Reverted: false,
		Meta:     dto.ReceiptMeta{},
		Outputs:  []*dto.Output{},
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
	assert.Equal(t, expectedReceipt.GasUsed, receipt.GasUsed)
	assert.Equal(t, expectedReceipt.GasPayer, receipt.GasPayer)
	assertHexOrDecimal256Equal(t, expectedReceipt.Paid, receipt.Paid)
	assertHexOrDecimal256Equal(t, expectedReceipt.Reward, receipt.Reward)
	assert.Equal(t, expectedReceipt.Reverted, receipt.Reverted)
	assert.Equal(t, expectedReceipt.Meta, receipt.Meta)
	assert.Equal(t, len(expectedReceipt.Outputs), len(receipt.Outputs))
}

func TestClient_InspectClauses(t *testing.T) {
	calldata := &dto.BatchCallData{}
	expectedResults := []*dto.CallResult{{
		Data:      "data",
		Events:    []*dto.Event{},
		Transfers: []*dto.Transfer{},
		GasUsed:   1000,
		Reverted:  false,
		VMError:   "no error",
	}}

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
	rawTx := &dto.RawTx{}
	expectedResult := &dto.SendTxResult{ID: &thor.Bytes32{0x01}}

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
	req := &dto.TransferFilter{}
	expectedTransfers := []*dto.FilteredTransfer{{
		Sender:    thor.Address{0x01},
		Recipient: thor.Address{0x02},
		Amount:    &math.HexOrDecimal256{},
		Meta:      dto.LogMeta{},
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
	assert.Equal(t, len(expectedTransfers), len(transfers))
	for i, expectedTransfer := range expectedTransfers {
		assert.Equal(t, expectedTransfer.Sender, transfers[i].Sender)
		assert.Equal(t, expectedTransfer.Recipient, transfers[i].Recipient)
		assertHexOrDecimal256Equal(t, expectedTransfer.Amount, transfers[i].Amount)
		assert.Equal(t, expectedTransfer.Meta, transfers[i].Meta)
	}
}

func TestClient_FilterEvents(t *testing.T) {
	req := &dto.EventFilter{}
	expectedEvents := []dto.FilteredEvent{{
		Address: thor.Address{0x01},
		Topics:  []*thor.Bytes32{{0x01}},
		Data:    "data",
		Meta:    dto.LogMeta{},
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
	expectedAccount := &dto.Account{
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
	assertHexOrDecimal256Equal(t, expectedAccount.Balance, account.Balance)
	assertHexOrDecimal256Equal(t, expectedAccount.Energy, account.Energy)
	assert.Equal(t, expectedAccount.HasCode, account.HasCode)
}

func TestClient_GetAccountCode(t *testing.T) {
	addr := thor.Address{0x01}
	expectedCodeRsp := &dto.GetCodeResult{Code: hexutil.Encode([]byte{0x01, 0x03})}

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
	expectedStorageRsp := &dto.GetStorageResult{Value: hexutil.Encode([]byte{0x01, 0x03})}

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

func TestClient_GetRawStorage(t *testing.T) {
	addr := thor.Address{0x01}
	key := thor.Bytes32{0x01}
	expectedStorageRsp := &dto.GetStorageResult{Value: hexutil.Encode([]byte{0x01, 0x03})}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts/"+addr.String()+"/storage/raw/"+key.String(), r.URL.Path)

		marshal, err := json.Marshal(expectedStorageRsp)
		require.NoError(t, err)

		w.Write(marshal)
	}))
	defer ts.Close()

	client := New(ts.URL)
	data, err := client.GetRawAccountStorage(&addr, &key, BestRevision)

	assert.NoError(t, err)
	assert.Equal(t, expectedStorageRsp.Value, data.Value)
}

func TestClient_GetExpandedBlock(t *testing.T) {
	blockID := "123"
	expectedBlock := &dto.ExpandedBlock{}

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
	expectedBlock := &dto.CollapsedBlock{
		BlockSummary: &dto.BlockSummary{
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
	var expectedBlock *dto.CollapsedBlock

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
	expectedTx := &dto.Transaction{ID: txID}

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
	expectedTx := &dto.RawTransaction{
		Meta: &dto.TxMeta{
			BlockID:        thor.Bytes32{0x01},
			BlockNumber:    1,
			BlockTimestamp: 123,
		},
		RawTx: dto.RawTx{Raw: hexutil.Encode([]byte{0x03})},
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
	expectedFeesHistory := &dto.FeesHistory{
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
	expectedFeesHistory := &dto.FeesHistory{
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
		assert.Equal(
			t,
			"/fees/history?blockCount="+fmt.Sprint(blockCount)+"&newestBlock="+newestBlock+"&rewardPercentiles="+strings.Join(rewardPercentilesStr, ","),
			r.URL.Path+"?"+r.URL.RawQuery,
		)

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
	expectedFeesPriority := &dto.FeesPriority{
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
	expectedPeers := []*dto.PeerStats{{
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

func TestClient_GetTxPool(t *testing.T) {
	t.Run("GetTxPoolWithTransactionIDs", func(t *testing.T) {
		expectedTxIDs := []*thor.Bytes32{
			{0x01, 0x02, 0x03},
			{0x04, 0x05, 0x06},
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/node/txpool", r.URL.Path)
			assert.Equal(t, "", r.URL.RawQuery)

			txIDsBytes, _ := json.Marshal(expectedTxIDs)
			w.Write(txIDsBytes)
		}))
		defer ts.Close()

		client := New(ts.URL)
		result, err := client.GetTxPool(nil)

		assert.NoError(t, err)
		assert.Equal(t, expectedTxIDs, result)
	})

	t.Run("GetTxPoolWithExpandedTransactions", func(t *testing.T) {
		expectedTxs := []*dto.Transaction{
			{ID: thor.Bytes32{0x01, 0x02, 0x03}},
			{ID: thor.Bytes32{0x04, 0x05, 0x06}},
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/node/txpool", r.URL.Path)
			assert.Equal(t, "expanded=true", r.URL.RawQuery)

			txsBytes, _ := json.Marshal(expectedTxs)
			w.Write(txsBytes)
		}))
		defer ts.Close()

		client := New(ts.URL)
		result, err := client.GetExpandedTxPool(nil)

		assert.NoError(t, err)
		assert.Equal(t, expectedTxs, result)
	})

	t.Run("GetTxPoolWithOriginFilter", func(t *testing.T) {
		origin := thor.Address{0x01, 0x02, 0x03}
		expectedTxIDs := []*thor.Bytes32{
			{0x01, 0x02, 0x03},
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/node/txpool", r.URL.Path)
			assert.Equal(t, "origin="+origin.String(), r.URL.RawQuery)

			txIDsBytes, _ := json.Marshal(expectedTxIDs)
			w.Write(txIDsBytes)
		}))
		defer ts.Close()

		client := New(ts.URL)
		result, err := client.GetTxPool(&origin)

		assert.NoError(t, err)
		assert.Equal(t, expectedTxIDs, result)
	})

	t.Run("GetTxPoolWithExpandedAndOrigin", func(t *testing.T) {
		origin := thor.Address{0x01, 0x02, 0x03}
		expectedTxs := []*dto.Transaction{
			{ID: thor.Bytes32{0x01, 0x02, 0x03}},
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/node/txpool", r.URL.Path)
			expectedQuery := "expanded=true&origin=" + origin.String()
			assert.Equal(t, expectedQuery, r.URL.RawQuery)

			txsBytes, _ := json.Marshal(expectedTxs)
			w.Write(txsBytes)
		}))
		defer ts.Close()

		client := New(ts.URL)
		result, err := client.GetExpandedTxPool(&origin)

		assert.NoError(t, err)
		assert.Equal(t, expectedTxs, result)
	})
}

func TestClient_GetTxPoolStatus(t *testing.T) {
	expectedStatus := &dto.TxPoolStatus{
		Amount: 42,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/node/txpool/status", r.URL.Path)

		statusBytes, _ := json.Marshal(expectedStatus)
		w.Write(statusBytes)
	}))
	defer ts.Close()

	client := New(ts.URL)
	status, err := client.GetTxPoolStatus()

	assert.NoError(t, err)
	assert.Equal(t, expectedStatus, status)
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
			function: func(client *Client) (*dto.Receipt, error) { return client.GetTransactionReceipt(&txID, "") },
		},
		{
			name: "InspectClauses",
			path: "/accounts/*",
			function: func(client *Client) ([]*dto.CallResult, error) {
				return client.InspectClauses(&dto.BatchCallData{}, "")
			},
		},
		{
			name: "SendTransaction",
			path: "/transactions",
			function: func(client *Client) (*dto.SendTxResult, error) {
				return client.SendTransaction(&dto.RawTx{})
			},
		},
		{
			name: "FilterTransfers",
			path: "/logs/transfer",
			function: func(client *Client) ([]*dto.FilteredTransfer, error) {
				return client.FilterTransfers(&dto.TransferFilter{})
			},
		},
		{
			name: "FilterEvents",
			path: "/logs/event",
			function: func(client *Client) ([]dto.FilteredEvent, error) {
				return client.FilterEvents(&dto.EventFilter{})
			},
		},
		{
			name:     "Account",
			path:     "/accounts/" + addr.String(),
			function: func(client *Client) (*dto.Account, error) { return client.GetAccount(&addr, "") },
		},
		{
			name:     "GetContractByteCode",
			path:     "/accounts/" + addr.String() + "/code",
			function: func(client *Client) (*dto.GetCodeResult, error) { return client.GetAccountCode(&addr, "") },
		},
		{
			name: "GetAccountStorage",
			path: "/accounts/" + addr.String() + "/storage/" + thor.Bytes32{}.String(),
			function: func(client *Client) (*dto.GetStorageResult, error) {
				return client.GetAccountStorage(&addr, &thor.Bytes32{}, BestRevision)
			},
		},
		{
			name:     "ExpandedBlock",
			path:     "/blocks/" + blockID + "?expanded=true",
			function: func(client *Client) (*dto.ExpandedBlock, error) { return client.GetExpandedBlock(blockID) },
		},
		{
			name:     "Block",
			path:     "/blocks/" + blockID,
			function: func(client *Client) (*dto.CollapsedBlock, error) { return client.GetBlock(blockID) },
		},
		{
			name: "Transaction",
			path: "/transactions/" + txID.String(),
			function: func(client *Client) (*dto.Transaction, error) {
				return client.GetTransaction(&txID, BestRevision, false)
			},
		},
		{
			name:     "Peers",
			path:     "/node/network/peers",
			function: func(client *Client) ([]*dto.PeerStats, error) { return client.GetPeers() },
		},
		{
			name:     "TxPool",
			path:     "/node/txpool",
			function: func(client *Client) (any, error) { return client.GetTxPool(nil) },
		},
		{
			name:     "TxPoolStatus",
			path:     "/node/txpool/status",
			function: func(client *Client) (*dto.TxPoolStatus, error) { return client.GetTxPoolStatus() },
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
