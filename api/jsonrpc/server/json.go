// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package server

import (
	"encoding/json"
	"errors"
)

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
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	// ponytail: Result holds a Go value (re-marshaled on write); geth stores
	// json.RawMessage. No functional impact — switch only if profiling says so.
	Result any        `json:"result,omitempty"`
	Error  *jsonError `json:"error,omitempty"`
}

type jsonError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *jsonError) Error() string { return e.Message }

// DataError lets a business error attach structured fields to error.data.
type DataError interface {
	error
	ErrorCode() int
	ErrorData() any
}

func errorResponse(id json.RawMessage, je *jsonError) *jsonrpcMessage {
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: id, Error: je}
}

func toJSONError(err error) *jsonError {
	var je *jsonError
	if errors.As(err, &je) {
		return je
	}
	out := &jsonError{Code: errcodeDefault, Message: err.Error()}
	var de DataError
	if errors.As(err, &de) {
		out.Code = de.ErrorCode()
		out.Data = de.ErrorData()
	}
	return out
}
