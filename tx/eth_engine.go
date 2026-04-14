// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package tx — EthereumTx ingestion engine.
//
// NormalizeEthereumTx turns raw bytes into a validated NormalizedEthereumTx for
// EthLegacy and EthTyped1559 transactions. The pipeline is:
//
//	Step 0: size check                    (< 128 KiB)
//	Step 1: detect tx kind                (Detector)
//	Step 2: RLP decode into typed struct  (Decoder)
//	Step 3: validate CHAIN_ID            (before ECDSA — cheap rejection)
//	Step 4: validate field ranges         (stateless)
//	Step 5: validate r, s bounds          (before ECDSA recovery)
//	Step 6: recover sender via secp256k1  (Recoverer)
//	Step 7: compute ethTxHash             (Hasher)
//	Step 8: assemble NormalizedEthereumTx (Normalizer)
//
// Everything below stays out of scope: mempool, replacement, base-fee checks,
// JSON-RPC formatting, block projection.

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
	// maxEthTxSize is the maximum serialised transaction size accepted by the engine.
	maxEthTxSize = 128 * 1024 // 128 KiB

	// max256BitLen is the maximum bit-length for 256-bit unsigned integer fields (value).
	max256BitLen = 256
)

// NormalizedEthereumTx is the engine's output — a fully decoded, validated, and
// sender-recovered Ethereum transaction ready for pool ingestion, execution, and
// RPC projection. It is the canonical boundary object passed between the engine
// and its consumers.
//
// Hash is computed once at engine time; consumers must never recompute or replace it.
// Raw is preserved so consumers can re-broadcast without re-encoding.
type NormalizedEthereumTx struct {
	// Identity
	Hash   thor.Bytes32 // ethTxHash = Keccak256(rawEthBytes)
	TxType Type         // TypeEthLegacy | TypeEthTyped1559

	// Sender recovered via secp256k1 ECDSA — not present on the wire.
	Sender thor.Address

	// Common fields
	Nonce    uint64
	GasLimit uint64
	To       *thor.Address // nil = contract creation
	Value    *big.Int
	Data     []byte
	ChainID  uint64

	// Fee fields — mutually exclusive per tx type.
	GasPrice             *big.Int // EthLegacy only; nil for EthTyped1559
	MaxFeePerGas         *big.Int // EthTyped1559 only; nil for EthLegacy
	MaxPriorityFeePerGas *big.Int // EthTyped1559 only; nil for EthLegacy

	// Raw Ethereum wire bytes, preserved for re-broadcast and hash verification.
	// For EthLegacy: the original 9-field RLP list (no 0x52 internal marker).
	// For EthTyped1559: the 0x02-prefixed full encoding.
	Raw []byte
}

// NormalizeEthereumTx validates and normalizes raw Ethereum transaction bytes.
//
// chainID is the CHAIN_ID this network enforces for EIP-155 replay protection.
// The caller is responsible for providing the correct CHAIN_ID from the fork config.
//
// On success, it returns a NormalizedEthereumTx.
// On failure, it returns an *EthTxError with a machine-readable EthTxErrorCode.
func NormalizeEthereumTx(rawBytes []byte, chainID uint64) (*NormalizedEthereumTx, error) {
	// Step 0: size check — before any parsing.
	if len(rawBytes) > maxEthTxSize {
		return nil, ethErr(EthErrOversized, "transaction exceeds 128 KiB limit")
	}

	// Step 1: detect tx kind.
	if len(rawBytes) == 0 {
		return nil, ethErr(EthErrMalformedEncoding, "empty transaction bytes")
	}
	first := rawBytes[0]

	switch {
	case first >= 0xC0:
		// EthLegacy: raw RLP list.
		return processEthLegacy(rawBytes, chainID)
	case first == TypeEthTyped1559:
		return processEth1559(rawBytes, chainID)
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

// TODO add more tests around the private methods

// processEthLegacy runs the full pipeline for an EthLegacy transaction.
func processEthLegacy(rawBytes []byte, chainID uint64) (*NormalizedEthereumTx, error) {
	// Step 2: RLP decode.
	// rlp.DecodeBytes enforces:
	//   - Canonical integer encoding (no leading zeros) for *big.Int and uint fields.
	//   - Exactly 9 fields (struct field count mismatch → error).
	//   - No trailing bytes after the outer RLP list.
	var body ethLegacyTransaction
	if err := rlp.DecodeBytes(rawBytes, &body); err != nil {
		return nil, wrapRLPErr(err)
	}

	// Step 3: validate CHAIN_ID before any ECDSA work.
	// EIP-155 requires V ≥ 35. V < 35 means a pre-EIP-155 signature (v=27 or v=28).
	if body.V.Sign() <= 0 || body.V.Cmp(big.NewInt(35)) < 0 {
		return nil, ethErr(EthErrEIP155Required, "pre-EIP-155 signatures (v=27, v=28) are not accepted")
	}
	wireChainID := body.chainID()
	if wireChainID.Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return nil, ethErrf(EthErrChainIDMismatch, "chain ID %s does not match expected %d", wireChainID, chainID)
	}

	// Step 4: validate field ranges.
	if err := validateEthLegacyFields(&body); err != nil {
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

	// Step 7: compute ethTxHash from original raw bytes.
	// rawBytes IS the canonical rawEthBytes for EthLegacy (no 0x52 marker).
	hash := thor.Keccak256(rawBytes)

	// Step 8: assemble.
	return &NormalizedEthereumTx{
		Hash:     hash,
		TxType:   TypeEthLegacy,
		Sender:   sender,
		Nonce:    body.Nonce,
		GasLimit: body.GasLimit,
		To:       cloneAddress(body.To),
		Value:    new(big.Int).Set(body.Value),
		Data:     bytes.Clone(body.Data),
		ChainID:  chainID,
		GasPrice: new(big.Int).Set(body.GasPrice),
		Raw:      rawBytes,
	}, nil
}

// processEth1559 runs the full pipeline for an EthTyped1559 transaction.
func processEth1559(rawBytes []byte, chainID uint64) (*NormalizedEthereumTx, error) {
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

	// Step 3: validate CHAIN_ID.
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

	// Step 8: assemble.
	return &NormalizedEthereumTx{
		Hash:                 hash,
		TxType:               TypeEthTyped1559,
		Sender:               sender,
		Nonce:                body.Nonce,
		GasLimit:             body.GasLimit,
		To:                   cloneAddress(body.To),
		Value:                new(big.Int).Set(body.Value),
		Data:                 bytes.Clone(body.Data),
		ChainID:              chainID,
		MaxFeePerGas:         new(big.Int).Set(body.MaxFeePerGas),
		MaxPriorityFeePerGas: new(big.Int).Set(body.MaxPriorityFeePerGas),
		Raw:                  rawBytes,
	}, nil
}

// validateEthLegacyFields performs stateless range checks on EthLegacy fields.
// Canonical RLP encoding (no leading zeros) is already enforced by the RLP decoder
// for *big.Int and uint fields.
func validateEthLegacyFields(t *ethLegacyTransaction) error {
	if t.GasPrice.Sign() <= 0 {
		return ethErr(EthErrInvalidField, "gasPrice must be > 0")
	}
	if t.GasLimit == 0 {
		return ethErr(EthErrInvalidField, "gasLimit must be > 0")
	}
	// RLP decodes *big.Int via SetBytes, which is always non-negative; only an upper-bound check is needed.
	if t.Value.BitLen() > max256BitLen {
		return ethErr(EthErrInvalidField, "value out of range [0, 2^256 - 1]")
	}
	// to: nil (contract creation) or exactly 20 bytes — enforced by *thor.Address `rlp:"nil"`.
	return nil
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
//
// Required invariants (encoding-hashing.md §6.4):
//
//	r ≠ 0 and r < N
//	s ≠ 0 and s ≤ N/2  (low-S mandatory)
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
// Used to keep NormalizedEthereumTx.To independent of the decoded body struct.
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
