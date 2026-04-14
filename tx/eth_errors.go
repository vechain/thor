// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import "fmt"

// EthTxErrorCode is a machine-readable code for Ethereum transaction validation failures.
// These codes are stable and safe for programmatic inspection by callers.
type EthTxErrorCode string

const (
	EthErrOversized                EthTxErrorCode = "oversized"
	EthErrUnsupportedTxType        EthTxErrorCode = "unsupported_tx_type"
	EthErrUnsupportedTxTypeEIP4844 EthTxErrorCode = "unsupported_tx_type_eip4844"
	EthErrMalformedEncoding        EthTxErrorCode = "malformed_encoding"
	EthErrNonCanonicalRLP          EthTxErrorCode = "non_canonical_rlp"
	EthErrInvalidField             EthTxErrorCode = "invalid_field"
	EthErrFeeInconsistency         EthTxErrorCode = "fee_inconsistency"
	EthErrEIP155Required           EthTxErrorCode = "eip155_required"
	EthErrChainIDMismatch          EthTxErrorCode = "chain_id_mismatch"
	EthErrInvalidR                 EthTxErrorCode = "invalid_r"
	EthErrInvalidS                 EthTxErrorCode = "invalid_s"
	EthErrHighSSignature           EthTxErrorCode = "high_s_signature"
	EthErrECDSARecoveryFailed      EthTxErrorCode = "ecdsa_recovery_failed"
	EthErrZeroSender               EthTxErrorCode = "zero_sender"
	EthErrAccessListUnsupported    EthTxErrorCode = "access_list_unsupported"
)

// EthTxError is a typed validation error returned by the Ethereum transaction engine.
// Callers can inspect Code for programmatic handling and Message for human-readable detail.
type EthTxError struct {
	Code    EthTxErrorCode
	Message string
}

func (e *EthTxError) Error() string {
	return fmt.Sprintf("eth tx [%s]: %s", e.Code, e.Message)
}

func ethErr(code EthTxErrorCode, msg string) *EthTxError {
	return &EthTxError{Code: code, Message: msg}
}

func ethErrf(code EthTxErrorCode, format string, args ...any) *EthTxError {
	return &EthTxError{Code: code, Message: fmt.Sprintf(format, args...)}
}
