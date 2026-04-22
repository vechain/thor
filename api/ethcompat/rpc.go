// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// JSON-RPC 2.0 standard error codes.
const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
	codeExecutionError = 3 // eth_call / eth_estimateGas revert
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func newErrResponse(id json.RawMessage, code int, msg string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// parseParams unmarshals the params array into a slice of raw messages.
func parseParams(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var params []json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	return params, nil
}

// paramString extracts the string at index i from a params slice.
func paramString(params []json.RawMessage, i int) (string, error) {
	if i >= len(params) {
		return "", fmt.Errorf("missing param at index %d", i)
	}
	var s string
	if err := json.Unmarshal(params[i], &s); err != nil {
		return "", err
	}
	return s, nil
}

// paramBool extracts the bool at index i from a params slice.
func paramBool(params []json.RawMessage, i int) (bool, error) {
	if i >= len(params) {
		return false, nil
	}
	var b bool
	if err := json.Unmarshal(params[i], &b); err != nil {
		return false, err
	}
	return b, nil
}

// writeJSON writes a JSON-RPC response to w.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
