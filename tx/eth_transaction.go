// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package tx — Ethereum transaction parsing.
//
// ParseEthTransaction validates raw Ethereum wire bytes and returns a *Transaction
// ready for pool ingestion and EVM execution. The parsing pipeline is:
//
//	Step 0: size check                   (< 64 KiB)
//	Step 1: detect tx kind               (type-byte switch)
//	Step 2: RLP decode into typed struct
//	Step 3: validate chain ID            (before ECDSA — cheap rejection)
//	Step 4: validate field ranges        (stateless)
//	Step 5: validate r, s bounds         (before ECDSA recovery)
//	Step 6: recover sender via secp256k1
//	Step 7: compute ethTxHash            (Keccak256 of raw wire bytes)
//	Step 8: construct *Transaction       (directly from parsed body — no re-decode)
//
// Out of scope: mempool replacement, base-fee checks, JSON-RPC formatting, block projection.

package tx

import (
	"bytes"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

const (
	// maxEthTxSize is the maximum serialised size accepted by ParseEthTransaction.
	// TODO: confirm alignment with Ethereum node mempool limits before mainnet activation.
	maxEthTxSize = 64 * 1024

	// max256BitLen is the maximum bit-length for 256-bit unsigned integer fields (value).
	max256BitLen = 256
)

// ParseEthTransaction parses and validates raw Ethereum wire-format bytes,
// recovers the sender via ECDSA, and returns a *Transaction with sender
// and ID pre-cached, ready for pool submission and EVM execution.
//
// chainID is the network's EIP-155 replay-protection chain ID. The caller is
// responsible for providing the correct value from the fork config.
//
// On failure it returns an *EthTxError with a machine-readable EthTxErrorCode.
func ParseEthTransaction(rawBytes []byte, chainID uint64) (*Transaction, error) {
	// Step 0: size check — before any parsing.
	if len(rawBytes) > maxEthTxSize {
		return nil, ethErr(EthErrOversized, "transaction exceeds 64 KiB limit")
	}

	// Step 1: detect tx kind.
	if len(rawBytes) == 0 {
		return nil, ethErr(EthErrMalformedEncoding, "empty transaction bytes")
	}
	first := rawBytes[0]

	switch {
	case first >= 0xC0:
		return nil, ethErr(EthErrUnsupportedTxType, "Ethereum legacy (type-0 RLP) transactions are not supported; use EIP-1559 (type 0x02)")
	case first == TypeEthTyped1559:
		return parseEth1559(rawBytes, chainID)
	case first == 0x01:
		return nil, ethErr(EthErrUnsupportedTxType, "EIP-2930 (type 0x01) transactions are not supported")
	case first == 0x03:
		return nil, ethErr(EthErrUnsupportedTxTypeEIP4844, "EIP-4844 (type 0x03) blob transactions are not supported")
	case first == TypeDynamicFee: // 0x51
		return nil, ethErr(EthErrUnsupportedTxType, "VeChainTx TypeDynamicFee (0x51) submitted to Ethereum endpoint")
	default:
		return nil, ethErrf(EthErrUnsupportedTxType, "unknown transaction type byte: 0x%02x", first)
	}
}

// parseEth1559 runs the full pipeline for an EIP-1559 typed transaction.
func parseEth1559(rawBytes []byte, chainID uint64) (*Transaction, error) {
	// rawBytes = 0x02 || rlpBody; strip the type prefix before RLP decoding.
	if len(rawBytes) < 2 {
		return nil, ethErr(EthErrMalformedEncoding, "EIP-1559 transaction too short")
	}
	rlpBody := rawBytes[1:]

	// Step 2: RLP decode.
	var body eth1559Transaction
	if err := rlp.DecodeBytes(rlpBody, &body); err != nil {
		return nil, wrapRLPErr(err)
	}

	// Step 3: validate chain ID.
	if body.ChainID.Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return nil, ethErrf(EthErrChainIDMismatch, "chain ID %s does not match expected %d", body.ChainID, chainID)
	}

	// Step 4: validate field ranges.
	if err := validateEth1559Fields(&body); err != nil {
		return nil, err
	}

	// Step 5: validate signature scalar bounds.
	if err := validateSigScalars(body.R, body.S); err != nil {
		return nil, err
	}

	// Step 6: recover sender.
	sender, err := recoverEthSender(body.ethSigningHash(), body.signature())
	if err != nil {
		return nil, err
	}

	// Step 7: compute ethTxHash from original raw bytes (includes 0x02 prefix).
	hash := thor.Keccak256(rawBytes)

	// Step 8: construct *Transaction directly from the parsed body — no re-decode.
	d := &eth1559TxData{
		chainID:     chainID,
		txNonce:     body.Nonce,
		maxPriority: new(big.Int).Set(body.MaxPriorityFeePerGas),
		maxFee:      new(big.Int).Set(body.MaxFeePerGas),
		gasLimit:    body.GasLimit,
		to:          cloneAddress(body.To),
		value:       new(big.Int).Set(body.Value),
		data:        bytes.Clone(body.Data),
		yParity:     body.YParity,
		r:           new(big.Int).Set(body.R),
		s:           new(big.Int).Set(body.S),
		ethHash:     hash,
		rawBytes:    bytes.Clone(rawBytes),
	}
	t := &Transaction{body: d}
	t.cache.id.Store(hash)
	t.cache.origin.Store(sender)
	t.cache.size.Store(uint64(len(rawBytes)))
	return t, nil
}

// validateEth1559Fields performs stateless range checks on EthTyped1559 fields.
func validateEth1559Fields(t *eth1559Transaction) error {
	if t.MaxFeePerGas.Sign() <= 0 {
		return ethErr(EthErrInvalidField, "maxFeePerGas must be > 0")
	}
	// RLP decodes *big.Int via SetBytes, which is always non-negative;
	// maxPriorityFeePerGas = 0 is valid (no tip), so no lower-bound check is needed here.
	// The consistency check below (≤ maxFeePerGas) provides the effective constraint.
	if t.MaxPriorityFeePerGas.Cmp(t.MaxFeePerGas) > 0 {
		return ethErr(EthErrFeeInconsistency, "maxPriorityFeePerGas must be ≤ maxFeePerGas")
	}
	if t.GasLimit == 0 {
		return ethErr(EthErrInvalidField, "gasLimit must be > 0")
	}
	// RLP decodes *big.Int via SetBytes, which is always non-negative; only an upper-bound check is needed.
	if t.Value.BitLen() > max256BitLen {
		return ethErr(EthErrInvalidField, "value out of range [0, 2^256 - 1]")
	}
	if t.YParity > 1 {
		return ethErrf(EthErrInvalidField, "yParity must be 0 or 1, got %d", t.YParity)
	}
	// TODO: implement EIP-2930 access list support — requires StateDB access list methods
	// (PrepareAccessList, AddressInAccessList, SlotInAccessList, AddAddressToAccessList,
	// AddSlotToAccessList), EIP-2929 warm/cold gas accounting in vm/gas_table.go, and
	// IsEIP2929/IsEIP2930 flags in vm.Rules.
	if len(t.AccessList) > 0 {
		return ethErr(EthErrAccessListUnsupported, "access lists are not yet supported")
	}
	return nil
}

// validateSigScalars checks the r and s signature components against the secp256k1 curve order.
// Low-S is mandatory to prevent signature malleability:
//
//	r ≠ 0 and r < N
//	s ≠ 0 and s ≤ N/2
func validateSigScalars(r, s *big.Int) error {
	if r.Sign() == 0 || r.Cmp(secp256k1N) >= 0 {
		return ethErr(EthErrInvalidR, "r must satisfy 0 < r < N")
	}
	if s.Sign() == 0 {
		return ethErr(EthErrInvalidS, "s must be > 0")
	}
	// Use the precomputed [32]byte array for the byte-level comparison that
	// EnforceSignatureLowS also uses, keeping the two checks consistent.
	sBytes := make([]byte, 32)
	s.FillBytes(sBytes)
	if bytes.Compare(sBytes, secp256k1HalfN[:]) > 0 {
		return ethErr(EthErrHighSSignature, "s exceeds secp256k1 half-order (high-S signatures are not accepted)")
	}
	return nil
}

// cloneAddress returns a heap copy of a *thor.Address, or nil if a is nil.
// Used to keep eth1559TxData.to independent of the decoded body struct.
func cloneAddress(a *thor.Address) *thor.Address {
	if a == nil {
		return nil
	}
	cpy := *a
	return &cpy
}

// recoverEthSender recovers the sender address from a Keccak256 signing hash and
// a normalised 65-byte [R(32) || S(32) || yParity(1)] signature blob.
//
// Returns EthErrECDSARecoveryFailed if recovery fails, EthErrZeroSender if the
// recovered address is the zero address.
func recoverEthSender(signingHash thor.Bytes32, sig []byte) (thor.Address, error) {
	pub, err := crypto.SigToPub(signingHash[:], sig)
	if err != nil {
		return thor.Address{}, ethErrf(EthErrECDSARecoveryFailed, "secp256k1 recovery failed: %v", err)
	}
	sender := thor.Address(crypto.PubkeyToAddress(*pub))
	if sender == (thor.Address{}) {
		return thor.Address{}, ethErr(EthErrZeroSender, "recovered sender is the zero address")
	}
	return sender, nil
}

// wrapRLPErr maps go-ethereum RLP decode errors to typed EthTxErrors.
// ErrCanonInt / ErrCanonSize → EthErrNonCanonicalRLP
// ErrMoreThanOneValue       → EthErrNonCanonicalRLP (trailing bytes)
// others                    → EthErrMalformedEncoding
//
// Note: when a canonical-encoding error occurs inside a struct field, the
// go-ethereum RLP decoder wraps it in an unexported *decodeError rather than
// returning the sentinel directly. The sentinel equality check therefore only
// matches at the stream level; the string check catches the wrapped form.
func wrapRLPErr(err error) *EthTxError {
	// errors.Is behaves identically to == here: go-ethereum's *decodeError does not
	// implement Unwrap(), so no chain traversal occurs. The sentinel cases match only
	// when the error is returned unwrapped (stream-level). The strings.Contains fallback
	// is what catches the wrapped form produced when the error originates inside a struct field.
	switch {
	case errors.Is(err, rlp.ErrCanonInt), errors.Is(err, rlp.ErrCanonSize):
		return ethErrf(EthErrNonCanonicalRLP, "non-canonical RLP encoding: %v", err)
	case errors.Is(err, rlp.ErrMoreThanOneValue):
		return ethErr(EthErrNonCanonicalRLP, "trailing bytes after RLP item")
	default:
		if strings.Contains(err.Error(), "non-canonical") {
			return ethErrf(EthErrNonCanonicalRLP, "non-canonical RLP encoding: %v", err)
		}
		return ethErrf(EthErrMalformedEncoding, "RLP decode error: %v", err)
	}
}
