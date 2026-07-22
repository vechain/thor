// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import "encoding/json"

const jsonrpcVersion = "2.0"

// JSON-RPC 2.0 error codes (see go-ethereum rpc/errors.go).
const (
	errcodeParse          = -32700
	errcodeInvalidRequest = -32600
	errcodeMethodNotFound = -32601
	errcodeInvalidParams  = -32602
	errcodeInternal       = -32603
	errcodeDefault        = -32000
)

type jsonrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonError      `json:"error,omitempty"`
}

type jsonError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *jsonError) Error() string { return e.Message }

// DataError lets a business error attach structured fields to error.data.
type DataError interface {
	error
	ErrorCode() int
	ErrorData() interface{}
}

func errorResponse(id json.RawMessage, je *jsonError) *jsonrpcMessage {
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: id, Error: je}
}

func toJSONError(err error) *jsonError {
	if je, ok := err.(*jsonError); ok {
		return je
	}
	je := &jsonError{Code: errcodeDefault, Message: err.Error()}
	if de, ok := err.(DataError); ok {
		je.Code = de.ErrorCode()
		je.Data = de.ErrorData()
	}
	return je
}
