// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/api/ethview"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func init() {
	register("eth_sendRawTransaction", handleSendRawTransaction)
	register("eth_getTransactionByHash", handleGetTransactionByHash)
	register("eth_getTransactionByBlockHashAndIndex", handleGetTransactionByBlockHashAndIndex)
	register("eth_getTransactionByBlockNumberAndIndex", handleGetTransactionByBlockNumberAndIndex)
	register("eth_getTransactionReceipt", handleGetTransactionReceipt)
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

// --- tx / receipt lookups ---------------------------------------------------

// handleGetTransactionByHash looks up a tx by its canonical hash (keccak256
// of the signed raw bytes for 0x02, or ID() for 0x00/0x51 — identical for
// legacy per chain/repository.go:162). Returns null when the hash is
// unknown. Multi-clause native txs surface tx_not_representable.
func handleGetTransactionByHash(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 1 {
		return nil, InvalidParams("expected [txHash]")
	}
	var hash thor.Bytes32
	if err := json.Unmarshal(args[0], &hash); err != nil {
		return nil, InvalidParams("tx hash: " + err.Error())
	}

	bestChain := s.repo.NewBestChain()
	trx, meta, err := bestChain.GetTransaction(hash)
	if err != nil {
		if s.repo.IsNotFound(err) {
			return nil, nil // eth convention: null for missing tx
		}
		return nil, InternalError(err)
	}

	view, err := buildTxViewMined(s, bestChain, trx, meta)
	if err != nil {
		return nil, FromEthViewError(err)
	}
	return view, nil
}

// handleGetTransactionByBlockHashAndIndex returns the tx at block `hash`,
// index `idx`. Out-of-bounds index -> null. Non-representable tx -> reason.
func handleGetTransactionByBlockHashAndIndex(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 2 {
		return nil, InvalidParams("expected [blockHash, index]")
	}
	var blockHash thor.Bytes32
	if err := json.Unmarshal(args[0], &blockHash); err != nil {
		return nil, InvalidParams("blockHash: " + err.Error())
	}
	idx, rerr := decodeHexIndex(args[1])
	if rerr != nil {
		return nil, rerr
	}
	return lookupTxByBlockIDAndIndex(s, blockHash, idx)
}

// handleGetTransactionByBlockNumberAndIndex is the block-number variant.
func handleGetTransactionByBlockNumberAndIndex(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 2 {
		return nil, InvalidParams("expected [blockTag, index]")
	}
	var tag BlockTag
	if err := json.Unmarshal(args[0], &tag); err != nil {
		return nil, InvalidParams("blockTag: " + err.Error())
	}
	idx, rerr := decodeHexIndex(args[1])
	if rerr != nil {
		return nil, rerr
	}
	_, summary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return nil, ToRPCError(err)
	}
	return lookupTxByBlockIDAndIndex(s, summary.Header.ID(), idx)
}

// handleGetTransactionReceipt returns the eth-shape receipt for the tx with
// the given canonical hash. Null for missing; non-representable surfaces the
// tx_not_representable reason.
func handleGetTransactionReceipt(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 1 {
		return nil, InvalidParams("expected [txHash]")
	}
	var hash thor.Bytes32
	if err := json.Unmarshal(args[0], &hash); err != nil {
		return nil, InvalidParams("tx hash: " + err.Error())
	}

	bestChain := s.repo.NewBestChain()
	trx, meta, err := bestChain.GetTransaction(hash)
	if err != nil {
		if s.repo.IsNotFound(err) {
			return nil, nil
		}
		return nil, InternalError(err)
	}
	receipt, err := bestChain.GetTransactionReceipt(hash)
	if err != nil {
		return nil, InternalError(err)
	}

	blockID, err := bestChain.GetBlockID(meta.BlockNum)
	if err != nil {
		return nil, InternalError(err)
	}
	blockSum, err := s.repo.GetBlockSummary(blockID)
	if err != nil {
		return nil, InternalError(err)
	}
	origin, err := trx.Origin()
	if err != nil {
		return nil, InternalError(err)
	}
	delegator, _ := trx.Delegator()

	blockNum := blockSum.Header.Number()
	// cumulativeGasUsed = sum of GasUsed across all receipts up to and
	// including this tx's index in the block.
	receipts, err := s.repo.GetBlockReceipts(blockID)
	if err != nil {
		return nil, InternalError(err)
	}
	var cumulativeGasUsed uint64
	for i := uint64(0); i <= meta.Index && i < uint64(len(receipts)); i++ {
		cumulativeGasUsed += receipts[i].GasUsed
	}

	txMeta := ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             uint32(meta.Index),
		Origin:            origin,
		Delegator:         delegator,
		EffectiveGasPrice: effectiveGasPriceForTx(trx, blockSum.Header.BaseFee()),
	}
	view, err := ethview.ProjectReceipt(trx, receipt, txMeta, cumulativeGasUsed, blockSum.Header.BaseFee())
	if err != nil {
		return nil, FromEthViewError(err)
	}
	return view, nil
}

// --- helpers shared by the lookup handlers ----------------------------------

// lookupTxByBlockIDAndIndex fetches block[idx] and projects it, honoring the
// same "null on out-of-range, reason on not-representable" contract the two
// byBlock*AndIndex methods share.
func lookupTxByBlockIDAndIndex(s *Server, blockID thor.Bytes32, idx uint64) (any, *RPCError) {
	blk, err := s.repo.GetBlock(blockID)
	if err != nil {
		if s.repo.IsNotFound(err) {
			return nil, nil
		}
		return nil, InternalError(err)
	}
	txs := blk.Transactions()
	if idx >= uint64(len(txs)) {
		return nil, nil
	}
	trx := txs[idx]

	origin, err := trx.Origin()
	if err != nil {
		return nil, InternalError(err)
	}
	delegator, _ := trx.Delegator()

	blockNum := blk.Header().Number()
	bid := blockID
	txMeta := ethview.TxMeta{
		BlockID:           &bid,
		BlockNumber:       &blockNum,
		Index:             uint32(idx),
		Origin:            origin,
		Delegator:         delegator,
		EffectiveGasPrice: effectiveGasPriceForTx(trx, blk.Header().BaseFee()),
	}
	view, err := ethview.ProjectTx(trx, txMeta)
	if err != nil {
		return nil, FromEthViewError(err)
	}
	return view, nil
}

// buildTxViewMined is the GetTransactionByHash companion — takes the
// already-fetched tx + chain.TxMeta, resolves the block summary for origin /
// timestamp / base fee, and calls ProjectTx. Non-representable surfaces the
// spec sentinel.
func buildTxViewMined(s *Server, bestChain *chain.Chain, trx *tx.Transaction, meta *chain.TxMeta) (*ethview.TransactionObject, error) {
	if meta == nil {
		return nil, errors.New("missing tx meta")
	}
	blockID, err := bestChain.GetBlockID(meta.BlockNum)
	if err != nil {
		return nil, err
	}
	blockSum, err := s.repo.GetBlockSummary(blockID)
	if err != nil {
		return nil, err
	}
	origin, err := trx.Origin()
	if err != nil {
		return nil, err
	}
	delegator, _ := trx.Delegator()

	blockNum := blockSum.Header.Number()
	txMeta := ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             uint32(meta.Index),
		Origin:            origin,
		Delegator:         delegator,
		EffectiveGasPrice: effectiveGasPriceForTx(trx, blockSum.Header.BaseFee()),
	}
	return ethview.ProjectTx(trx, txMeta)
}

// effectiveGasPriceForTx computes the per-gas price that should appear on
// the projected object. For 0x00 this needs the legacy base gas price from
// state (deferred until callexec lands in Task 10); for now we supply
// maxFeePerGas / 0 as a best-effort approximation for mined projections so
// wallets see a non-zero value. Pending 0x02/0x51 is handled by ethview
// (falls back to maxFeePerGas when EffectiveGasPrice is nil).
func effectiveGasPriceForTx(trx *tx.Transaction, baseFee *big.Int) *big.Int {
	switch trx.Type() {
	case tx.TypeEthDynamicFee, tx.TypeDynamicFee:
		if baseFee == nil {
			return new(big.Int).Set(trx.MaxFeePerGas())
		}
		// effective = min(maxFeePerGas, baseFee + maxPriorityFeePerGas)
		cap := trx.MaxFeePerGas()
		rate := new(big.Int).Add(baseFee, trx.MaxPriorityFeePerGas())
		if rate.Cmp(cap) > 0 {
			rate = cap
		}
		return rate
	default:
		// 0x00 legacy gasPrice is bgp × (255 + gasPriceCoef) / 255 where
		// bgp comes from state (builtin.Params, KeyLegacyTxBaseGasPrice).
		// That lookup belongs in a state-aware helper; callers that hit
		// this path before Phase 4 wiring will see a zero gasPrice, which
		// is better than a runtime panic.
		return new(big.Int)
	}
}

func decodeHexIndex(raw json.RawMessage) (uint64, *RPCError) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, InvalidParams("index must be a hex string")
	}
	n, err := parseHexUint64(s)
	if err != nil {
		return 0, InvalidParams(err.Error())
	}
	return n, nil
}

