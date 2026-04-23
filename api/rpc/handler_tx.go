// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func init() {
	register("eth_sendRawTransaction", handleSendRawTransaction)
}

// handleSendRawTransaction accepts a single hex-encoded raw transaction and
// routes it through tx.UnmarshalBinary (which dispatches on the first byte:
// 0xC0+ -> legacy RLP list, 0x51 -> VeChain dynamic-fee, 0x02 -> EIP-1559).
// On successful pool admission the canonical txid (keccak256 of signed RLP)
// is returned as a DATA string; admission errors map to data.reason codes
// per spec §3 D3 / §7.2.
func handleSendRawTransaction(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []hexutil.Bytes
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParams("params must be [hex-encoded-raw-tx]")
	}
	if len(args) != 1 {
		return nil, InvalidParams("expected exactly one argument")
	}
	raw := []byte(args[0])
	if len(raw) == 0 {
		return nil, InvalidParams("raw tx is empty")
	}

	trx := new(tx.Transaction)
	if err := trx.UnmarshalBinary(raw); err != nil {
		// Unknown first byte / malformed RLP — treat type errors as a
		// business reason so wallets see tx_type_not_supported instead of
		// a raw decode message.
		if errors.Is(err, tx.ErrTxTypeNotSupported) {
			return nil, ReasonError(ReasonTxTypeNotSupported, err.Error())
		}
		return nil, InvalidParams("raw tx decode: " + err.Error())
	}

	if err := s.pool.StrictlyAdd(trx); err != nil {
		return nil, mapTxPoolError(err)
	}
	return trx.CanonicalTxID(), nil
}

// mapTxPoolError converts a txpool / runtime admission error into a *RPCError
// with a canonical data.reason. The mapping is substring-based because the
// underlying errors are stringy (badTxError{"<msg>"} and txRejectedError{
// "<msg>"}) and inspecting wrapped sentinels would require broad exports
// from txpool. Substring matching is acceptable here because every matched
// string is the authoritative in-tree message — tests pin them in place and
// a repo-level grep flags drift.
//
// Unmatched errors fall through to tx_validation_failed (spec §3 D3), which
// keeps the wallet UX coherent without introducing ad-hoc reasons.
func mapTxPoolError(err error) *RPCError {
	// Sentinel errors from tx / txpool.
	if errors.Is(err, tx.ErrTxTypeNotSupported) {
		return ReasonError(ReasonTxTypeNotSupported, err.Error())
	}

	msg := err.Error()

	switch {
	case contains(msg, "eth tx type not supported before INTERSTELLAR"),
		contains(msg, "transaction type not supported"):
		return ReasonError(ReasonTxTypeNotSupported, msg)

	case contains(msg, "eth tx chain id mismatch"),
		contains(msg, "chain tag mismatch"):
		return ReasonError(ReasonChainIDMismatch, msg)

	case contains(msg, "size too large"):
		return ReasonError(ReasonOversizedData, msg)

	case contains(msg, "access list not supported"),
		contains(msg, "eth tx access list"):
		return ReasonError(ReasonAccessListNotSupported, msg)

	case contains(msg, "insufficient energy"),
		contains(msg, "insufficient balance"),
		contains(msg, "insufficient funds"):
		return ReasonError(ReasonInsufficientFunds, msg)

	case contains(msg, "known tx"),
		contains(msg, "already known"):
		return ReasonError(ReasonTxKnown, msg)

	case contains(msg, "intrinsic gas"):
		return ReasonError(ReasonIntrinsicGasTooLow, msg)

	case contains(msg, "maxFeePerGas"),
		contains(msg, "fee cap"),
		contains(msg, "fee cap too low"):
		return ReasonError(ReasonFeeCapTooLow, msg)

	case contains(msg, "tip above fee cap"),
		contains(msg, "priority fee"):
		return ReasonError(ReasonTipAboveFeeCap, msg)

	case contains(msg, "underpriced"):
		return ReasonError(ReasonTxUnderpriced, msg)

	case contains(msg, "S value is out of range"),
		contains(msg, "invalid signature"):
		return ReasonError(ReasonTxValidationFailed, msg)
	}

	// Fallback — pool full, not executable, unclassified badTx.
	// Recognize either the typed wrapper or the rendered prefix so callers
	// that forward an already-stringified error still flow through.
	if txpool.IsTxRejected(err) || txpool.IsBadTx(err) ||
		strings.HasPrefix(msg, "tx rejected:") || strings.HasPrefix(msg, "bad tx:") {
		return ReasonError(ReasonTxValidationFailed, msg)
	}
	return InternalError(err)
}

// contains is strings.Contains with a case-sensitive match; kept local so
// the hot substring table above reads as a switch.
func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
