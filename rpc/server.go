// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxRequestBodySize = 2 * 1024 * 1024 // 2 MB

// TODO: revisit this limit — 10 is conservative; raise once the performance
// profile of synchronous batch processing is better understood.
const maxBatchRequests = 10

// Server is an HTTP handler that implements the Ethereum JSON-RPC protocol.
// It supports both single and batch requests.
type Server struct {
	d *Dispatcher
}

// New creates a new Server backed by the given Dispatcher.
func New(d *Dispatcher) *Server {
	return &Server{d: d}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		// CORS preflight handled by the gorilla/handlers CORS middleware applied externally.
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "only POST requests are accepted", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		writeJSON(w, ErrResponse(nil, CodeParseError, "failed to read request body"))
		return
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		writeJSON(w, ErrResponse(nil, CodeParseError, "empty request body"))
		return
	}

	if trimmed[0] == '[' {
		s.handleBatch(w, trimmed)
	} else {
		s.handleSingle(w, trimmed)
	}
}

func (s *Server) handleSingle(w http.ResponseWriter, body []byte) {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, ErrResponse(nil, CodeParseError, "invalid JSON: "+err.Error()))
		return
	}
	writeJSON(w, s.d.dispatch(req))
}

func (s *Server) handleBatch(w http.ResponseWriter, body []byte) {
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		writeJSON(w, ErrResponse(nil, CodeParseError, "invalid JSON array: "+err.Error()))
		return
	}
	if len(raws) == 0 {
		writeJSON(w, ErrResponse(nil, CodeInvalidParams, "empty batch"))
		return
	}
	if len(raws) > maxBatchRequests {
		writeJSON(w, ErrResponse(nil, CodeInvalidParams, fmt.Sprintf("batch size %d exceeds maximum of %d", len(raws), maxBatchRequests)))
		return
	}

	responses := make([]Response, len(raws))
	for i, raw := range raws {
		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			responses[i] = ErrResponse(nil, CodeParseError, "invalid request in batch: "+err.Error())
			continue
		}
		responses[i] = s.d.dispatch(req)
	}
	writeJSON(w, responses)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
