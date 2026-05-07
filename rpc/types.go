// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import "encoding/json"

// Request is a JSON-RPC 2.0 request object.
type Request struct {
	Jsonrpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
}

// Response is a JSON-RPC 2.0 response object.
type Response struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeServerError    = -32000 // execution error, revert, etc.
)

// ErrResponse constructs a JSON-RPC error response.
func ErrResponse(id json.RawMessage, code int, msg string) Response {
	return Response{
		Jsonrpc: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
}

// ErrResponseWithData constructs a JSON-RPC error response with an extra data field.
func ErrResponseWithData(id json.RawMessage, code int, msg, data string) Response {
	return Response{
		Jsonrpc: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg, Data: data},
	}
}

// OkResponse constructs a successful JSON-RPC response.
func OkResponse(id json.RawMessage, result any) Response {
	data, _ := json.Marshal(result)
	return Response{
		Jsonrpc: "2.0",
		ID:      id,
		Result:  data,
	}
}
