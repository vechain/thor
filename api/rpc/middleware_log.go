// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"fmt"
	"time"

	"github.com/vechain/thor/v2/log"
)

// defaultLogger is the fallback when Config.Logger is not provided. It maps
// to the project-wide log root with a "pkg=eth-rpc" tag so the line still
// stands out in mixed-output stdout.
var defaultLogger = log.WithContext("pkg", "eth-rpc")

const paramsPreviewLimit = 200

const (
	shortHexMinLen    = 13
	shortHexPrefixLen = 7
	shortHexSuffixLen = 4
)

// logExchange emits one structured slog.Info line per /rpc exchange. Guarded
// by s.cfg.EnableReqLogger (nil-safe; nil pointer means disabled). Output goes
// to s.cfg.Logger when set, otherwise to the package default.
//
// referer is the inbound Referer header — set by browsers on page-level
// fetches (e.g. http://localhost:8080/), suppressed by browser extensions
// like MetaMask, opt-in for non-browser clients (txblast sets it).
//
// origin is the inbound Origin header — set by browsers on every cross-origin
// POST as a CORS requirement, including from extensions. This is what makes
// MetaMask traffic identifiable: chrome-extension://nkbihfbeogaeaoehlefnkodbefgpgknn
// (Chrome) or moz-extension://<uuid> (Firefox). Empty for non-browser callers.
func (s *Server) logExchange(method, referer, origin string, env rpcResponse, body []byte, latency time.Duration) {
	if s.cfg.EnableReqLogger == nil || !s.cfg.EnableReqLogger.Load() {
		return
	}
	attrs := []any{
		"method", method,
		"code", errorCode(env),
		"latency_ms", float64(latency.Microseconds()) / 1000.0,
		"referer", referer,
		"origin", origin,
		"params_preview", paramsPreview(body),
	}
	if env.Error != nil {
		attrs = append(attrs, "reason", reasonFromError(env.Error))
	} else if preview := resultSummary(method, env.Result); preview != "" {
		attrs = append(attrs, "result_preview", preview)
	}
	lg := s.cfg.Logger
	if lg == nil {
		lg = defaultLogger
	}
	lg.Info("eth-rpc", attrs...)
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
// "...(truncated)" when the input is longer. The caller passes req.Params
// when the envelope parses (so we skip the "jsonrpc"/"method"/"id" keys);
// on parse or batch errors it passes the raw body as a fallback so the
// preview still captures context.
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
