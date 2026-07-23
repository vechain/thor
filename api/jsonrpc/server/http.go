// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// maxBatchItems caps requests per batch (geth's default), bounding work per
// call beyond the outer body-size limit.
//
// TODO: /rpc currently sits under the REST global 200KB body limit
// (cmd/thor/httpserver/api_server.go: router.Use(HandleRequestBodyLimit(...)),
// which has no path exclusion), so a batch is capped by that long before it
// hits maxBatchItems. Revisit the middleware setup: give /rpc its own body cap
// (path-exclude it from the REST limit + an internal MaxBytesReader) and align
// maxBatchItems with that cap.
const maxBatchItems = 1000

// ServeHTTP implements http.Handler, dispatching single or batch JSON-RPC requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
		return
	}
	ctx := r.Context()
	trimmed := bytes.TrimLeft(body, " \t\r\n")

	if len(trimmed) > 0 && trimmed[0] == '[' {
		var msgs []jsonrpcMessage
		if err := json.Unmarshal(body, &msgs); err != nil {
			writeJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: "invalid batch"}))
			return
		}
		if len(msgs) == 0 {
			writeJSON(w, errorResponse(nil, &jsonError{Code: errcodeInvalidRequest, Message: "empty batch"}))
			return
		}
		if len(msgs) > maxBatchItems {
			writeJSON(w, errorResponse(nil, &jsonError{
				Code:    errcodeInvalidRequest,
				Message: fmt.Sprintf("batch too large, max %d requests", maxBatchItems),
			}))
			return
		}
		// Skip nil responses: notifications (no id) get no reply.
		resps := make([]*jsonrpcMessage, 0, len(msgs))
		for i := range msgs {
			if resp := s.handleMsg(ctx, &msgs[i]); resp != nil {
				resps = append(resps, resp)
			}
		}
		if len(resps) == 0 {
			return
		}
		writeJSON(w, resps)
		return
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		writeJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
		return
	}
	if resp := s.handleMsg(ctx, &msg); resp != nil {
		writeJSON(w, resp)
	}
}

func writeJSON(w http.ResponseWriter, resp any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
