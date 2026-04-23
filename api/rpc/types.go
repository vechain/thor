// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package rpc implements Thor's Ethereum-compatible JSON-RPC namespace
// (eth_*). It exposes a single POST /rpc endpoint that parses JSON-RPC 2.0
// envelopes, dispatches method names through a string table, and delegates
// business logic to existing Thor modules (api/fees, api/events, api/accounts,
// txpool, chain.Repository, logdb) via internal service wrappers.
package rpc

import (
	"encoding/json"
)

// Config carries the knobs the rpc server needs. Fields mirror the subset of
// APIConfig relevant to eth_* handling; Phase 5 wires the real APIConfig into
// this struct at StartAPIServer time.
type Config struct {
	// BodyLimit bounds the JSON-RPC request body in bytes. Zero means use
	// the hard-coded default.
	BodyLimit int64

	// LogsLimit caps the number of log entries returned by eth_getLogs.
	LogsLimit uint64

	// APIBacktraceLimit caps both the eth_getLogs block range and the
	// eth_feeHistory backtrack window.
	APIBacktraceLimit int

	// CallGasLimit caps the gas any eth_call / eth_estimateGas execution
	// may consume.
	CallGasLimit uint64

	// PriorityIncreasePercentage mirrors APIConfig.PriorityIncreasePercentage
	// and is passed to the feesvc helpers used by eth_maxPriorityFeePerGas /
	// eth_gasPrice.
	PriorityIncreasePercentage int
}

const defaultBodyLimit int64 = 200 * 1024 // 200 KiB, matches REST default.

func (c Config) bodyLimit() int64 {
	if c.BodyLimit > 0 {
		return c.BodyLimit
	}
	return defaultBodyLimit
}

// rpcRequest is the wire shape of a JSON-RPC 2.0 single request.
//
// Params is left as RawMessage so handlers can json.Unmarshal into the shape
// they expect (a []json.RawMessage of arity N, or a positional object per
// method).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// rpcResponse is the wire shape of a JSON-RPC 2.0 single response.
// omitempty on Result would strip legitimate `null` results (e.g. from
// eth_getTransactionByHash on a missing tx), so we use a pointer-like shape
// with MarshalJSON below to decide which of Result / Error to emit.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  any             `json:"-"`
	Error   *RPCError       `json:"-"`
	ID      json.RawMessage `json:"id"`

	// resultSet is true when Result was explicitly provided (including a
	// nil / json-null value). Distinguishes "null success" from "error".
	resultSet bool
}

// MarshalJSON renders the response per JSON-RPC 2.0: exactly one of "result"
// or "error" is present. A nil Result with resultSet=true still emits
// "result": null.
func (r rpcResponse) MarshalJSON() ([]byte, error) {
	if r.Error != nil {
		return json.Marshal(&struct {
			JSONRPC string          `json:"jsonrpc"`
			Error   *RPCError       `json:"error"`
			ID      json.RawMessage `json:"id"`
		}{r.JSONRPC, r.Error, envelopeID(r.ID)})
	}
	return json.Marshal(&struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  any             `json:"result"`
		ID      json.RawMessage `json:"id"`
	}{r.JSONRPC, r.Result, envelopeID(r.ID)})
}

// envelopeID ensures an unset ID is rendered as JSON null rather than empty.
func envelopeID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}
