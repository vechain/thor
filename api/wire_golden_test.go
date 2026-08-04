// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api_test

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

// golden marshals v and compares it against api/testdata/<name>.json. Missing
// baselines are generated on first run and must be spot-checked against
// api/doc/thor.yaml before being committed.
func golden(t *testing.T, name string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	path := filepath.Join("testdata", name+".json")
	want, rerr := os.ReadFile(path)
	if os.IsNotExist(rerr) {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
		t.Fatalf("golden %s generated; verify against api/doc/thor.yaml then re-run", path)
	}
	require.NoError(t, rerr)
	require.Equal(t, string(want), string(data), "wire drift: %s", name)
}

// ---- account ----

func TestGoldenAccount(t *testing.T) {
	golden(t, "account", &api.Account{
		Balance: (*math.HexOrDecimal256)(big.NewInt(100)),
		Energy:  (*math.HexOrDecimal256)(big.NewInt(200)),
		HasCode: true,
	})
}

func TestGoldenGetCodeResult(t *testing.T) {
	golden(t, "get_code_result", &api.GetCodeResult{Code: "0x616263"})
}

func TestGoldenGetStorageResult(t *testing.T) {
	golden(t, "get_storage_result", &api.GetStorageResult{Value: "0x616263"})
}

func TestGoldenGetRawStorageResponse(t *testing.T) {
	golden(t, "get_raw_storage_response", &api.GetRawStorageResponse{Value: "0x616263"})
}

func TestGoldenCallResult(t *testing.T) {
	golden(t, "call_result", &api.CallResult{
		Data: "0x616263",
		Events: []*api.Event{{
			Address: thor.BytesToAddress([]byte{1}),
			Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{2})},
			Data:    "0x616263",
		}},
		Transfers: []*api.Transfer{{
			Sender:    thor.BytesToAddress([]byte{3}),
			Recipient: thor.BytesToAddress([]byte{4}),
			Amount:    (*math.HexOrDecimal256)(big.NewInt(500)),
		}},
		GasUsed:  21000,
		Reverted: true,
		VMError:  "execution reverted",
	})
}

func TestGoldenBatchCallResults(t *testing.T) {
	golden(t, "batch_call_results", &api.BatchCallResults{
		{
			Data: "0x616263",
			Events: []*api.Event{{
				Address: thor.BytesToAddress([]byte{1}),
				Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{2})},
				Data:    "0x616263",
			}},
			Transfers: []*api.Transfer{{
				Sender:    thor.BytesToAddress([]byte{3}),
				Recipient: thor.BytesToAddress([]byte{4}),
				Amount:    (*math.HexOrDecimal256)(big.NewInt(500)),
			}},
			GasUsed:  21000,
			Reverted: true,
			VMError:  "execution reverted",
		},
	})
}

// ---- admin ----

func TestGoldenLogStatus(t *testing.T) {
	golden(t, "log_status", &api.LogStatus{Enabled: true})
}

func TestGoldenToggleStatus(t *testing.T) {
	golden(t, "toggle_status", &api.ToggleStatus{Enabled: true, TTLSeconds: 30})
	golden(t, "toggle_status_min", &api.ToggleStatus{})
}

func TestGoldenHealthStatus(t *testing.T) {
	bestBlockTime := time.Unix(1700000000, 0).UTC()
	nodeMaster := "0x0000000000000000000000000000000000000001"
	beneficiary := "0x0000000000000000000000000000000000000002"
	golden(t, "health_status", &api.HealthStatus{
		Healthy:              true,
		BestBlockTime:        &bestBlockTime,
		PeerCount:            5,
		IsNetworkProgressing: true,
		NodeMaster:           &nodeMaster,
		Beneficiary:          &beneficiary,
	})
}

func TestGoldenLogLevelResponse(t *testing.T) {
	golden(t, "log_level_response", &api.LogLevelResponse{CurrentLevel: "debug"})
}

// ---- blocks ----

func fullBlockSummary() *api.JSONBlockSummary {
	return &api.JSONBlockSummary{
		Number:        1,
		ID:            thor.BytesToBytes32([]byte{1}),
		Size:          2,
		ParentID:      thor.BytesToBytes32([]byte{3}),
		Timestamp:     4,
		GasLimit:      5,
		Beneficiary:   thor.BytesToAddress([]byte{6}),
		GasUsed:       7,
		TotalScore:    8,
		TxsRoot:       thor.BytesToBytes32([]byte{9}),
		TxsFeatures:   10,
		StateRoot:     thor.BytesToBytes32([]byte{11}),
		ReceiptsRoot:  thor.BytesToBytes32([]byte{12}),
		COM:           true,
		Signer:        thor.BytesToAddress([]byte{13}),
		IsTrunk:       true,
		IsFinalized:   true,
		BaseFeePerGas: (*math.HexOrDecimal256)(big.NewInt(14)),
	}
}

func fullJSONOutput() *api.JSONOutput {
	addr := thor.BytesToAddress([]byte{20})
	return &api.JSONOutput{
		ContractAddress: &addr,
		Events: []*api.JSONEvent{{
			Address: thor.BytesToAddress([]byte{21}),
			Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{22})},
			Data:    "0x616263",
		}},
		Transfers: []*api.JSONTransfer{{
			Sender:    thor.BytesToAddress([]byte{23}),
			Recipient: thor.BytesToAddress([]byte{24}),
			Amount:    (*math.HexOrDecimal256)(big.NewInt(25)),
		}},
	}
}

func fullEmbeddedTx() *api.JSONEmbeddedTx {
	coef := uint8(1)
	delegator := thor.BytesToAddress([]byte{30})
	dependsOn := thor.BytesToBytes32([]byte{31})
	to := thor.BytesToAddress([]byte{32})
	return &api.JSONEmbeddedTx{
		ID:                   thor.BytesToBytes32([]byte{33}),
		Type:                 1,
		ChainTag:             2,
		BlockRef:             "0x0000000000000003",
		Expiration:           4,
		Clauses:              api.Clauses{{To: &to, Value: (*math.HexOrDecimal256)(big.NewInt(5)), Data: "0x616263"}},
		GasPriceCoef:         &coef,
		MaxFeePerGas:         (*math.HexOrDecimal256)(big.NewInt(6)),
		MaxPriorityFeePerGas: (*math.HexOrDecimal256)(big.NewInt(7)),
		Gas:                  8,
		Origin:               thor.BytesToAddress([]byte{34}),
		Delegator:            &delegator,
		Nonce:                math.HexOrDecimal64(9),
		DependsOn:            &dependsOn,
		Size:                 10,
		GasUsed:              11,
		GasPayer:             thor.BytesToAddress([]byte{35}),
		Paid:                 (*math.HexOrDecimal256)(big.NewInt(12)),
		Reward:               (*math.HexOrDecimal256)(big.NewInt(13)),
		Reverted:             true,
		Outputs:              []*api.JSONOutput{fullJSONOutput()},
	}
}

func TestGoldenJSONBlockSummary(t *testing.T) {
	golden(t, "block_summary", fullBlockSummary())
	golden(t, "block_summary_min", &api.JSONBlockSummary{})
}

func TestGoldenJSONRawBlockSummary(t *testing.T) {
	golden(t, "raw_block_summary", &api.JSONRawBlockSummary{Raw: "0x616263"})
}

func TestGoldenJSONCollapsedBlock(t *testing.T) {
	golden(t, "collapsed_block", &api.JSONCollapsedBlock{
		JSONBlockSummary: fullBlockSummary(),
		Transactions:     []thor.Bytes32{thor.BytesToBytes32([]byte{15})},
	})
	golden(t, "collapsed_block_min", &api.JSONCollapsedBlock{})
}

func TestGoldenJSONEmbeddedTx(t *testing.T) {
	golden(t, "embedded_tx", fullEmbeddedTx())
	golden(t, "embedded_tx_min", &api.JSONEmbeddedTx{})
}

func TestGoldenJSONExpandedBlock(t *testing.T) {
	golden(t, "expanded_block", &api.JSONExpandedBlock{
		JSONBlockSummary: fullBlockSummary(),
		Transactions:     []*api.JSONEmbeddedTx{fullEmbeddedTx()},
	})
	golden(t, "expanded_block_min", &api.JSONExpandedBlock{})
}

// ---- common ----

func TestGoldenEvent(t *testing.T) {
	golden(t, "event", &api.Event{
		Address: thor.BytesToAddress([]byte{1}),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{2})},
		Data:    "0x616263",
	})
}

func TestGoldenTransfer(t *testing.T) {
	golden(t, "transfer", &api.Transfer{
		Sender:    thor.BytesToAddress([]byte{1}),
		Recipient: thor.BytesToAddress([]byte{2}),
		Amount:    (*math.HexOrDecimal256)(big.NewInt(3)),
	})
}

func TestGoldenClause(t *testing.T) {
	to := thor.BytesToAddress([]byte{1})
	golden(t, "clause", &api.Clause{
		To:    &to,
		Value: (*math.HexOrDecimal256)(big.NewInt(2)),
		Data:  "0x616263",
	})
}

func TestGoldenLogMeta(t *testing.T) {
	ti, li := uint32(7), uint32(8)
	golden(t, "log_meta", &api.LogMeta{
		BlockID:        thor.BytesToBytes32([]byte{1}),
		BlockNumber:    2,
		BlockTimestamp: 3,
		TxID:           thor.BytesToBytes32([]byte{4}),
		TxOrigin:       thor.BytesToAddress([]byte{5}),
		ClauseIndex:    6,
		TxIndex:        &ti,
		LogIndex:       &li,
	})
	golden(t, "log_meta_min", &api.LogMeta{})
}

// ---- debug ----

func TestGoldenStorageRangeResult(t *testing.T) {
	key := thor.BytesToBytes32([]byte{1})
	value := thor.BytesToBytes32([]byte{2})
	nextKey := thor.BytesToBytes32([]byte{3})
	golden(t, "storage_range_result", &api.StorageRangeResult{
		Storage: api.StorageMap{
			"0x01": api.StorageEntry{Key: &key, Value: &value},
		},
		NextKey: &nextKey,
	})
}

// ---- events ----

func TestGoldenFilteredEvent(t *testing.T) {
	ti, li := uint32(7), uint32(8)
	topic := thor.BytesToBytes32([]byte{2})
	golden(t, "filtered_event", &api.FilteredEvent{
		Address: thor.BytesToAddress([]byte{1}),
		Topics:  []*thor.Bytes32{&topic},
		Data:    "0x616263",
		Meta: api.LogMeta{
			BlockID:        thor.BytesToBytes32([]byte{3}),
			BlockNumber:    4,
			BlockTimestamp: 5,
			TxID:           thor.BytesToBytes32([]byte{6}),
			TxOrigin:       thor.BytesToAddress([]byte{9}),
			ClauseIndex:    10,
			TxIndex:        &ti,
			LogIndex:       &li,
		},
	})
	golden(t, "filtered_event_min", &api.FilteredEvent{})
}

// ---- fees ----

func TestGoldenFeesHistory(t *testing.T) {
	golden(t, "fees_history", &api.FeesHistory{
		OldestBlock:   thor.BytesToBytes32([]byte{1}),
		BaseFeePerGas: []*hexutil.Big{(*hexutil.Big)(big.NewInt(2))},
		GasUsedRatio:  []float64{0.5},
		Reward:        [][]*hexutil.Big{{(*hexutil.Big)(big.NewInt(3))}},
	})
	golden(t, "fees_history_min", &api.FeesHistory{})
}

func TestGoldenFeesPriority(t *testing.T) {
	golden(t, "fees_priority", &api.FeesPriority{
		MaxPriorityFeePerGas: (*hexutil.Big)(big.NewInt(1)),
	})
}

// ---- node ----

func TestGoldenNodeStatus(t *testing.T) {
	golden(t, "node_status", &api.Status{Amount: 1})
}

func TestGoldenPeerStats(t *testing.T) {
	golden(t, "peer_stats", &api.PeerStats{
		Name:        "peer1",
		BestBlockID: thor.BytesToBytes32([]byte{1}),
		TotalScore:  2,
		PeerID:      "enode://abc",
		NetAddr:     "127.0.0.1:11235",
		Inbound:     true,
		Duration:    3,
	})
}

// ---- subscriptions ----

func TestGoldenBlockMessage(t *testing.T) {
	golden(t, "block_message", &api.BlockMessage{
		Number:        1,
		ID:            thor.BytesToBytes32([]byte{2}),
		Size:          3,
		ParentID:      thor.BytesToBytes32([]byte{4}),
		Timestamp:     5,
		GasLimit:      6,
		Beneficiary:   thor.BytesToAddress([]byte{7}),
		GasUsed:       8,
		BaseFeePerGas: (*math.HexOrDecimal256)(big.NewInt(9)),
		TotalScore:    10,
		TxsRoot:       thor.BytesToBytes32([]byte{11}),
		TxsFeatures:   12,
		StateRoot:     thor.BytesToBytes32([]byte{13}),
		ReceiptsRoot:  thor.BytesToBytes32([]byte{14}),
		COM:           true,
		Signer:        thor.BytesToAddress([]byte{15}),
		Transactions:  []thor.Bytes32{thor.BytesToBytes32([]byte{16})},
		Obsolete:      true,
	})
	golden(t, "block_message_min", &api.BlockMessage{})
}

func TestGoldenTransferMessage(t *testing.T) {
	ti, li := uint32(1), uint32(2)
	golden(t, "transfer_message", &api.TransferMessage{
		Sender:    thor.BytesToAddress([]byte{1}),
		Recipient: thor.BytesToAddress([]byte{2}),
		Amount:    (*math.HexOrDecimal256)(big.NewInt(3)),
		Meta: api.LogMeta{
			BlockID:        thor.BytesToBytes32([]byte{4}),
			BlockNumber:    5,
			BlockTimestamp: 6,
			TxID:           thor.BytesToBytes32([]byte{7}),
			TxOrigin:       thor.BytesToAddress([]byte{8}),
			ClauseIndex:    9,
			TxIndex:        &ti,
			LogIndex:       &li,
		},
		Obsolete: true,
	})
	golden(t, "transfer_message_min", &api.TransferMessage{})
}

func TestGoldenEventMessage(t *testing.T) {
	ti, li := uint32(1), uint32(2)
	golden(t, "event_message", &api.EventMessage{
		Address: thor.BytesToAddress([]byte{1}),
		Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{2})},
		Data:    "0x616263",
		Meta: api.LogMeta{
			BlockID:        thor.BytesToBytes32([]byte{3}),
			BlockNumber:    4,
			BlockTimestamp: 5,
			TxID:           thor.BytesToBytes32([]byte{6}),
			TxOrigin:       thor.BytesToAddress([]byte{7}),
			ClauseIndex:    8,
			TxIndex:        &ti,
			LogIndex:       &li,
		},
		Obsolete: true,
	})
	golden(t, "event_message_min", &api.EventMessage{})
}

func TestGoldenBeatMessage(t *testing.T) {
	golden(t, "beat_message", &api.BeatMessage{
		Number:      1,
		ID:          thor.BytesToBytes32([]byte{2}),
		ParentID:    thor.BytesToBytes32([]byte{3}),
		Timestamp:   4,
		TxsFeatures: 5,
		Bloom:       "0x616263",
		K:           6,
		Obsolete:    true,
	})
}

func TestGoldenBeat2Message(t *testing.T) {
	golden(t, "beat2_message", &api.Beat2Message{
		Number:        1,
		ID:            thor.BytesToBytes32([]byte{2}),
		ParentID:      thor.BytesToBytes32([]byte{3}),
		Timestamp:     4,
		TxsFeatures:   5,
		BaseFeePerGas: (*math.HexOrDecimal256)(big.NewInt(6)),
		GasLimit:      7,
		Bloom:         "0x616263",
		K:             8,
		Obsolete:      true,
	})
	golden(t, "beat2_message_min", &api.Beat2Message{})
}

func TestGoldenPendingTxIDMessage(t *testing.T) {
	golden(t, "pending_txid_message", &api.PendingTxIDMessage{ID: thor.BytesToBytes32([]byte{1})})
}

// ---- transactions ----

func TestGoldenTransaction(t *testing.T) {
	coef := uint8(1)
	delegator := thor.BytesToAddress([]byte{2})
	dependsOn := thor.BytesToBytes32([]byte{3})
	to := thor.BytesToAddress([]byte{4})
	golden(t, "transaction", &transactions.Transaction{
		ID:                   thor.BytesToBytes32([]byte{5}),
		Type:                 1,
		ChainTag:             2,
		BlockRef:             "0x0000000000000003",
		Expiration:           6,
		Clauses:              api.Clauses{{To: &to, Value: (*math.HexOrDecimal256)(big.NewInt(7)), Data: "0x616263"}},
		GasPriceCoef:         &coef,
		Gas:                  8,
		MaxFeePerGas:         (*math.HexOrDecimal256)(big.NewInt(9)),
		MaxPriorityFeePerGas: (*math.HexOrDecimal256)(big.NewInt(10)),
		Origin:               thor.BytesToAddress([]byte{11}),
		Delegator:            &delegator,
		Nonce:                math.HexOrDecimal64(12),
		DependsOn:            &dependsOn,
		Size:                 13,
		Meta: &api.TxMeta{
			BlockID:        thor.BytesToBytes32([]byte{14}),
			BlockNumber:    15,
			BlockTimestamp: 16,
		},
	})
	golden(t, "transaction_min", &transactions.Transaction{})
}

func TestGoldenRawTransaction(t *testing.T) {
	golden(t, "raw_transaction", &api.RawTransaction{
		RawTx: api.RawTx{Raw: "0x616263"},
		Meta: &api.TxMeta{
			BlockID:        thor.BytesToBytes32([]byte{1}),
			BlockNumber:    2,
			BlockTimestamp: 3,
		},
	})
}

func TestGoldenTxMeta(t *testing.T) {
	golden(t, "tx_meta", &api.TxMeta{
		BlockID:        thor.BytesToBytes32([]byte{1}),
		BlockNumber:    2,
		BlockTimestamp: 3,
	})
}

func TestGoldenReceiptMeta(t *testing.T) {
	golden(t, "receipt_meta", &api.ReceiptMeta{
		BlockID:        thor.BytesToBytes32([]byte{1}),
		BlockNumber:    2,
		BlockTimestamp: 3,
		TxID:           thor.BytesToBytes32([]byte{4}),
		TxOrigin:       thor.BytesToAddress([]byte{5}),
	})
}

func TestGoldenReceipt(t *testing.T) {
	contractAddr := thor.BytesToAddress([]byte{1})
	golden(t, "receipt", &api.Receipt{
		Type:     1,
		GasUsed:  2,
		GasPayer: thor.BytesToAddress([]byte{3}),
		Paid:     (*math.HexOrDecimal256)(big.NewInt(4)),
		Reward:   (*math.HexOrDecimal256)(big.NewInt(5)),
		Reverted: true,
		Meta: api.ReceiptMeta{
			BlockID:        thor.BytesToBytes32([]byte{6}),
			BlockNumber:    7,
			BlockTimestamp: 8,
			TxID:           thor.BytesToBytes32([]byte{9}),
			TxOrigin:       thor.BytesToAddress([]byte{10}),
		},
		Outputs: []*api.Output{{
			ContractAddress: &contractAddr,
			Events: []*api.Event{{
				Address: thor.BytesToAddress([]byte{11}),
				Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{12})},
				Data:    "0x616263",
			}},
			Transfers: []*api.Transfer{{
				Sender:    thor.BytesToAddress([]byte{13}),
				Recipient: thor.BytesToAddress([]byte{14}),
				Amount:    (*math.HexOrDecimal256)(big.NewInt(15)),
			}},
		}},
	})
	golden(t, "receipt_min", &api.Receipt{})
}

func TestGoldenOutput(t *testing.T) {
	contractAddr := thor.BytesToAddress([]byte{1})
	golden(t, "output", &api.Output{
		ContractAddress: &contractAddr,
		Events: []*api.Event{{
			Address: thor.BytesToAddress([]byte{2}),
			Topics:  []thor.Bytes32{thor.BytesToBytes32([]byte{3})},
			Data:    "0x616263",
		}},
		Transfers: []*api.Transfer{{
			Sender:    thor.BytesToAddress([]byte{4}),
			Recipient: thor.BytesToAddress([]byte{5}),
			Amount:    (*math.HexOrDecimal256)(big.NewInt(6)),
		}},
	})
}

func TestGoldenSendTxResult(t *testing.T) {
	id := thor.BytesToBytes32([]byte{1})
	golden(t, "send_tx_result", &api.SendTxResult{ID: &id})
}

// ---- transfers ----

func TestGoldenFilteredTransfer(t *testing.T) {
	ti, li := uint32(7), uint32(8)
	golden(t, "filtered_transfer", &api.FilteredTransfer{
		Sender:    thor.BytesToAddress([]byte{1}),
		Recipient: thor.BytesToAddress([]byte{2}),
		Amount:    (*math.HexOrDecimal256)(big.NewInt(3)),
		Meta: api.LogMeta{
			BlockID:        thor.BytesToBytes32([]byte{4}),
			BlockNumber:    5,
			BlockTimestamp: 6,
			TxID:           thor.BytesToBytes32([]byte{9}),
			TxOrigin:       thor.BytesToAddress([]byte{10}),
			ClauseIndex:    11,
			TxIndex:        &ti,
			LogIndex:       &li,
		},
	})
	golden(t, "filtered_transfer_min", &api.FilteredTransfer{})
}

// ---- request-type unmarshal regression (R4 guardrail) ----

func TestTransferCriteriaUnmarshalCasing(t *testing.T) {
	for _, in := range []string{
		`{"txOrigin":"0x0000000000000000000000000000000000000001","sender":"0x0000000000000000000000000000000000000002","recipient":"0x0000000000000000000000000000000000000003"}`,
		`{"TxOrigin":"0x0000000000000000000000000000000000000001","Sender":"0x0000000000000000000000000000000000000002","Recipient":"0x0000000000000000000000000000000000000003"}`,
	} {
		var c logdb.TransferCriteria
		require.NoError(t, json.Unmarshal([]byte(in), &c))
		require.Equal(t, thor.BytesToAddress([]byte{1}), *c.TxOrigin)
		require.Equal(t, thor.BytesToAddress([]byte{2}), *c.Sender)
		require.Equal(t, thor.BytesToAddress([]byte{3}), *c.Recipient)
	}
}
