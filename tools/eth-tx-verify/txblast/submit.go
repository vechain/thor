// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// refererValue is sent on every outbound HTTP request so the server-side
// per-request log (eth-rpc.log via --api-eth-rpc-log-file) can attribute
// traffic to txblast and distinguish it from MetaMask (chrome-extension://…)
// and the verification DApp (http://localhost:8080/).
const refererValue = "txblast/eth-tx-verify"

// SubmitResult is returned by SubmitREST and SubmitRPC.
type SubmitResult struct {
	TxID string // hex "0x..." on success
	Err  error  // non-nil on HTTP / decoding / RPC error
}

// ReceiptResult is returned by ReceiptREST and ReceiptRPC.
type ReceiptResult struct {
	Block uint64 // block number (0 if not found)
	Found bool
	Err   error
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      int             `json:"id"`
}

type rpcResponse struct {
	JSONRPC string            `json:"jsonrpc"`
	Result  json.RawMessage   `json:"result,omitempty"`
	Error   *rpcResponseError `json:"error,omitempty"`
	ID      int               `json:"id"`
}

type rpcResponseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// callRPC posts a JSON-RPC 2.0 request to {base}/rpc and returns the raw result.
func callRPC(ctx context.Context, base, method string, params any) (json.RawMessage, error) {
	paramBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("callRPC marshal params: %w", err)
	}
	env := rpcRequest{JSONRPC: "2.0", Method: method, Params: paramBytes, ID: 1}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("callRPC marshal envelope: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", refererValue)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var r rpcResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("callRPC decode response: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("rpc %d: %s data=%s", r.Error.Code, r.Error.Message, string(r.Error.Data))
	}
	return r.Result, nil
}

// SubmitREST posts {"raw":"0x<hex>"} to {base}/transactions.
// Returns SubmitResult{TxID} on 200 success; SubmitResult{Err} on any failure.
func SubmitREST(ctx context.Context, base string, raw []byte) SubmitResult {
	body, _ := json.Marshal(map[string]string{"raw": hexutil.Encode(raw)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/transactions", bytes.NewReader(body))
	if err != nil {
		return SubmitResult{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", refererValue)
	resp, err := httpClient.Do(req)
	if err != nil {
		return SubmitResult{Err: err}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return SubmitResult{Err: fmt.Errorf("rest submit: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))}
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return SubmitResult{Err: err}
	}
	return SubmitResult{TxID: r.ID}
}

// SubmitRPC posts eth_sendRawTransaction to {base}/rpc.
// Decodes the JSON-RPC 2.0 envelope; Err is populated if the envelope carries
// an error or if the HTTP call fails.
func SubmitRPC(ctx context.Context, base string, raw []byte) SubmitResult {
	result, err := callRPC(ctx, base, "eth_sendRawTransaction", []string{hexutil.Encode(raw)})
	if err != nil {
		return SubmitResult{Err: err}
	}
	var txid string
	if err := json.Unmarshal(result, &txid); err != nil {
		return SubmitResult{Err: err}
	}
	return SubmitResult{TxID: txid}
}

// ReceiptREST queries {base}/transactions/{txid}/receipt.
// A 200 with null body or 404 means not yet mined → Found=false, no error.
func ReceiptREST(ctx context.Context, base, txID string) ReceiptResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/transactions/"+txID+"/receipt", nil)
	if err != nil {
		return ReceiptResult{Err: err}
	}
	req.Header.Set("Referer", refererValue)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ReceiptResult{Err: err}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 404 {
		return ReceiptResult{Found: false}
	}
	if resp.StatusCode/100 != 2 {
		return ReceiptResult{Err: fmt.Errorf("rest receipt: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))}
	}
	if strings.TrimSpace(string(b)) == "null" {
		return ReceiptResult{Found: false}
	}
	var r struct {
		Meta struct {
			BlockNumber uint64 `json:"blockNumber"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return ReceiptResult{Err: err}
	}
	return ReceiptResult{Block: r.Meta.BlockNumber, Found: true}
}

// ReceiptRPC queries eth_getTransactionReceipt via JSON-RPC.
// result == null → Found=false. Populate Block from result.blockNumber (hex).
func ReceiptRPC(ctx context.Context, base, txID string) ReceiptResult {
	result, err := callRPC(ctx, base, "eth_getTransactionReceipt", []string{txID})
	if err != nil {
		return ReceiptResult{Err: err}
	}
	if len(result) == 0 || strings.TrimSpace(string(result)) == "null" {
		return ReceiptResult{Found: false}
	}
	var recv struct {
		BlockNumber string `json:"blockNumber"`
	}
	if err := json.Unmarshal(result, &recv); err != nil {
		return ReceiptResult{Err: err}
	}
	blk, err := hexutil.DecodeUint64(recv.BlockNumber)
	if err != nil {
		return ReceiptResult{Err: err}
	}
	return ReceiptResult{Block: blk, Found: true}
}

// GetEthChainID calls eth_chainId at {base}/rpc; returns the uint64 value.
// Used at startup to derive 0x02 ChainID and to fill the DApp's wallet_addEthereumChain payload.
func GetEthChainID(ctx context.Context, base string) (uint64, error) {
	result, err := callRPC(ctx, base, "eth_chainId", []any{})
	if err != nil {
		return 0, err
	}
	var hexStr string
	if err := json.Unmarshal(result, &hexStr); err != nil {
		return 0, err
	}
	return hexutil.DecodeUint64(hexStr)
}

// GetEthNonce calls eth_getTransactionCount(addr, "latest") at {base}/rpc.
// Used by the EthNonce manager at startup and recovery.
func GetEthNonce(ctx context.Context, base, addr string) (uint64, error) {
	result, err := callRPC(ctx, base, "eth_getTransactionCount", []string{addr, "latest"})
	if err != nil {
		return 0, err
	}
	var hexStr string
	if err := json.Unmarshal(result, &hexStr); err != nil {
		return 0, err
	}
	return hexutil.DecodeUint64(hexStr)
}
