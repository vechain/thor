// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"errors"
	"fmt"

	"github.com/vechain/thor/v2/api/ethview"
)

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// CodeServerError is the implementation-defined band used for
	// business-logic failures. We always pair it with a data.reason string.
	CodeServerError = -32000
)

// data.reason whitelist per spec §7.2. Every member is either emitted by a
// handler in this package or reserved for future use (nonceTooLow,
// nonceGap). Do not add ad-hoc strings at call sites.
const (
	ReasonExecutionReverted            = "execution_reverted"
	ReasonBlockNotCanonical            = "block_not_canonical"
	ReasonTxKnown                      = "tx_known"
	ReasonTxUnderpriced                = "tx_underpriced"
	ReasonNonceTooLow                  = "nonce_too_low" // reserved; not currently emitted
	ReasonNonceGap                     = "nonce_gap"     // reserved; not currently emitted
	ReasonInsufficientFunds            = "insufficient_funds"
	ReasonChainIDMismatch              = "chain_id_mismatch"
	ReasonTxTypeNotSupported           = "tx_type_not_supported"
	ReasonAccessListNotSupported       = "access_list_not_supported"
	ReasonIntrinsicGasTooLow           = "intrinsic_gas_too_low"
	ReasonGasUintOverflow              = "gas_uint_overflow"
	ReasonGasCapExceeded               = "gas_cap_exceeded"
	ReasonFeeCapTooLow                 = "fee_cap_too_low"
	ReasonTipAboveFeeCap               = "tip_above_fee_cap"
	ReasonOversizedData                = "oversized_data"
	ReasonLogRangeTooLarge             = "log_range_too_large"
	ReasonTxValidationFailed           = "tx_validation_failed"
	ReasonTxNotRepresentable           = "tx_not_representable"
	ReasonBlockContainsNonRepresentable = "block_contains_tx_not_representable"
	ReasonStateOverridesNotSupported   = "state_overrides_not_supported"
)

// RPCError is the JSON-RPC 2.0 error object, implementing Go's error
// interface so handlers can `return nil, someRPCError` naturally.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// --- Constructors for standard codes -----------------------------------------

// ParseError is returned when the request body is not syntactically valid
// JSON.
func ParseError(detail string) *RPCError {
	return &RPCError{Code: CodeParseError, Message: "parse error: " + detail}
}

// InvalidRequest is returned when the envelope is JSON but malformed
// (missing or bad jsonrpc / method fields), or when batch requests are
// rejected up-front.
func InvalidRequest(detail string) *RPCError {
	return &RPCError{Code: CodeInvalidRequest, Message: detail}
}

// MethodNotFound is returned when the method name is not registered in the
// dispatch table (or is explicitly unsupported, e.g. eth_getProof).
func MethodNotFound(method string) *RPCError {
	return &RPCError{Code: CodeMethodNotFound, Message: "method not found: " + method}
}

// InvalidParams is returned when param decoding or shape validation fails
// (arity mismatch, wrong type, out-of-range, mutually-exclusive fields).
func InvalidParams(detail string) *RPCError {
	return &RPCError{Code: CodeInvalidParams, Message: "invalid params: " + detail}
}

// InternalError wraps an unexpected server-side failure (typically a
// repository / state read that should not fail during normal operation).
func InternalError(err error) *RPCError {
	return &RPCError{Code: CodeInternalError, Message: "internal error: " + err.Error()}
}

// --- Reason-coded errors -----------------------------------------------------

// ReasonError returns a CodeServerError with a canonical data.reason set.
// The reason must be one of the constants above; ad-hoc strings are a bug.
func ReasonError(reason, message string) *RPCError {
	return &RPCError{
		Code:    CodeServerError,
		Message: message,
		Data:    map[string]string{"reason": reason},
	}
}

// FromEthViewError translates the ethview sentinel errors into their canonical
// JSON-RPC reason codes. Returns nil when err is nil, and bubbles the raw
// error up as an InternalError for anything unknown so an unexpected sentinel
// is still debuggable on the client.
func FromEthViewError(err error) *RPCError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ethview.ErrNotRepresentable):
		return ReasonError(ReasonTxNotRepresentable, err.Error())
	case errors.Is(err, ethview.ErrBlockContainsNonRepresentable):
		return ReasonError(ReasonBlockContainsNonRepresentable, err.Error())
	default:
		return InternalError(err)
	}
}
