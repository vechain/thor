// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"fmt"
	"time"

	"github.com/vechain/thor/v2/log"
)

var logger = log.WithContext("pkg", "eth-rpc")

const paramsPreviewLimit = 200

const (
	shortHexMinLen    = 13
	shortHexPrefixLen = 7
	shortHexSuffixLen = 4
)

// logExchange emits one structured slog.Info line per /rpc exchange. Guarded
// by s.cfg.EnableReqLogger (nil-safe; nil pointer means disabled).
func (s *Server) logExchange(method string, env rpcResponse, body []byte, latency time.Duration) {
	if s.cfg.EnableReqLogger == nil || !s.cfg.EnableReqLogger.Load() {
		return
	}
	attrs := []any{
		"method", method,
		"code", errorCode(env),
		"latency_ms", float64(latency.Microseconds()) / 1000.0,
		"params_preview", paramsPreview(body),
	}
	if env.Error != nil {
		attrs = append(attrs, "reason", reasonFromError(env.Error))
	} else if preview := resultSummary(method, env.Result); preview != "" {
		attrs = append(attrs, "result_preview", preview)
	}
	logger.Info("eth-rpc", attrs...)
}

func errorCode(env rpcResponse) int {
	if env.Error != nil {
		return env.Error.Code
	}
	return 0
}

// reasonFromError extracts a human-readable reason string from an RPCError.
// It expects Data to be map[string]string or map[string]any with key "reason";
// anything else falls through to a code-derived reason.
func reasonFromError(err *RPCError) string {
	switch data := err.Data.(type) {
	case map[string]string:
		if r, ok := data["reason"]; ok {
			return r
		}
	case map[string]any:
		if r, ok := data["reason"]; ok {
			if s, ok := r.(string); ok {
				return s
			}
		}
	}
	// Fallback for standard codes.
	switch err.Code {
	case CodeParseError:
		return "parse_error"
	case CodeInvalidRequest:
		return "invalid_request"
	case CodeMethodNotFound:
		return "method_not_found"
	case CodeInvalidParams:
		return "invalid_params"
	}
	return "internal_error"
}

// paramsPreview returns up to paramsPreviewLimit bytes of raw, appending
// "...(truncated)" when the input is longer. When the request body cannot be
// parsed, callers pass the raw body here so the preview still captures context.
func paramsPreview(raw []byte) string {
	if len(raw) <= paramsPreviewLimit {
		return string(raw)
	}
	return string(raw[:paramsPreviewLimit]) + "...(truncated)"
}

// resultSummary returns a short human-readable preview for methods where the
// result is a single scalar (txid, block number, chain id, gas price). For all
// other methods an empty string is returned; callers omit the field in that case.
func resultSummary(method string, result any) string {
	switch method {
	case "eth_sendRawTransaction":
		if h, ok := result.(string); ok {
			return shortHex(h)
		}
		if s, ok := result.(fmt.Stringer); ok {
			return shortHex(s.String())
		}
	case "eth_blockNumber", "eth_chainId", "eth_gasPrice":
		if h, ok := result.(string); ok {
			return h
		}
		if s, ok := result.(fmt.Stringer); ok {
			return s.String()
		}
	}
	return ""
}

// shortHex turns 0x<hex> into 0xabc...def (first 7 + "..." + last 4 chars).
// Short values (≤13 chars) are returned unchanged.
func shortHex(s string) string {
	if len(s) <= shortHexMinLen {
		return s
	}
	return s[:shortHexPrefixLen] + "..." + s[len(s)-shortHexSuffixLen:]
}
