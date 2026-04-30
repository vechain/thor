// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

// EthBuilder constructs and signs Ethereum wire-format transactions.
//
// Unlike the VeChain Builder, which produces an unsigned tx that the caller
// signs separately, EthBuilder signs at Build time: Ethereum's signature is
// embedded in the wire encoding, so the private key must be present when the
// transaction is finalised.
//
// The EIP-2930 / EIP-1559 access list field is always encoded as empty.
// VeChain rejects non-empty access lists at the engine level, so no setter
// is provided — any such transaction would be rejected immediately.
//
// Unset *big.Int fields (GasPrice, MaxFeePerGas, MaxPriorityFeePerGas, Value)
// default to zero. Build returns an error if NormalizeEthereumTx rejects the
// encoded bytes (e.g. gasPrice == 0, gasLimit == 0).
type EthBuilder struct {
	txType               Type
	chainID              uint64
	nonce                uint64
	maxFeePerGas         *big.Int
	maxPriorityFeePerGas *big.Int
	gasLimit             uint64
	to                   *thor.Address // nil = contract creation
	value                *big.Int
	data                 []byte
}

// NewEthBuilder creates an EthBuilder for EthTyped1559 transactions.
// Panics if txType is not TypeEthTyped1559.
func NewEthBuilder(txType Type) *EthBuilder {
	if txType != TypeEthTyped1559 {
		panic(fmt.Sprintf("EthBuilder: unsupported tx type 0x%02x; use TypeEthTyped1559", txType))
	}
	return &EthBuilder{txType: txType}
}

// ChainID sets the Ethereum EIP-155 / EIP-1559 replay-protection chain ID.
// This is embedded in the signing preimage (EthTyped1559)
// and verified by NormalizeEthereumTx.
func (b *EthBuilder) ChainID(id uint64) *EthBuilder {
	b.chainID = id
	return b
}

// Nonce sets the transaction nonce.
func (b *EthBuilder) Nonce(n uint64) *EthBuilder {
	b.nonce = n
	return b
}

// MaxFeePerGas sets the maxFeePerGas field.
// NormalizeEthereumTx requires maxFeePerGas > 0.
// The value is copied; mutating v after this call has no effect.
func (b *EthBuilder) MaxFeePerGas(v *big.Int) *EthBuilder {
	b.maxFeePerGas = cloneBigInt(v)
	return b
}

// MaxPriorityFeePerGas sets the maxPriorityFeePerGas field. Only meaningful for TypeEthTyped1559.
// NormalizeEthereumTx requires maxPriorityFeePerGas ≤ maxFeePerGas.
// The value is copied; mutating v after this call has no effect.
func (b *EthBuilder) MaxPriorityFeePerGas(v *big.Int) *EthBuilder {
	b.maxPriorityFeePerGas = cloneBigInt(v)
	return b
}

// GasLimit sets the gas limit.
// NormalizeEthereumTx requires gasLimit > 0.
func (b *EthBuilder) GasLimit(n uint64) *EthBuilder {
	b.gasLimit = n
	return b
}

// To sets the recipient address. Pass nil for contract creation.
func (b *EthBuilder) To(to *thor.Address) *EthBuilder {
	b.to = to
	return b
}

// Value sets the amount of VET to transfer (in wei-equivalent smallest units).
// The value is copied; mutating v after this call has no effect.
func (b *EthBuilder) Value(v *big.Int) *EthBuilder {
	b.value = cloneBigInt(v)
	return b
}

// Data sets the transaction call data.
func (b *EthBuilder) Data(d []byte) *EthBuilder {
	b.data = d
	return b
}

// BuildRaw signs the transaction and returns Ethereum wire-format bytes: 0x02 || RLP(body).
//
// BuildRaw does not validate field semantics; NormalizeEthereumTx (called by
// Build) performs those checks. Call BuildRaw directly when you need the raw
// bytes — for example to broadcast to an Ethereum node, or to construct
// negative-path test cases by tampering specific fields before passing them
// to NormalizeEthereumTx.
func (b *EthBuilder) BuildRaw(key *ecdsa.PrivateKey) ([]byte, error) {
	if key == nil {
		return nil, fmt.Errorf("EthBuilder: private key must not be nil")
	}
	return b.buildEth1559Wire(key)
}

// Build signs, encodes, normalises, and wraps the transaction as a *Transaction
// ready for pool submission, EVM execution, and block inclusion.
//
// Returns an error if signing fails or if NormalizeEthereumTx rejects the
// encoded bytes (e.g. gasPrice == 0, gasLimit == 0, chainID mismatch, low-S violation).
func (b *EthBuilder) Build(key *ecdsa.PrivateKey) (*Transaction, error) {
	rawBytes, err := b.BuildRaw(key)
	if err != nil {
		return nil, err
	}
	norm, err := NormalizeEthereumTx(rawBytes, b.chainID)
	if err != nil {
		return nil, err
	}
	return NewEthereumTransaction(norm), nil
}

// buildEth1559Wire produces EIP-1559 signed wire bytes: 0x02 || RLP(body).
//
// EIP-1559 signing preimage:
//
//	Keccak256(0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas,
//	                        gasLimit, to, value, data, accessList]))
//
// accessList is always nil here — it encodes as the RLP empty list 0xC0.
// VeChain's engine rejects non-empty access lists, so there is no setter.
func (b *EthBuilder) buildEth1559Wire(key *ecdsa.PrivateKey) ([]byte, error) {
	sigHash := ethKeccakPrefixedRlpHash(TypeEthTyped1559, &eth1559SigningBody{
		ChainID:              new(big.Int).SetUint64(b.chainID),
		Nonce:                b.nonce,
		MaxPriorityFeePerGas: bigOrZero(b.maxPriorityFeePerGas),
		MaxFeePerGas:         bigOrZero(b.maxFeePerGas),
		GasLimit:             b.gasLimit,
		To:                   b.to,
		Value:                bigOrZero(b.value),
		Data:                 b.data,
		// AccessList: nil — encodes as the RLP empty list 0xC0.
	})

	sig, err := crypto.Sign(sigHash[:], key)
	if err != nil {
		return nil, fmt.Errorf("EthBuilder: signing failed: %w", err)
	}

	bodyRLP, err := rlp.EncodeToBytes(&eth1559Transaction{
		ChainID:              new(big.Int).SetUint64(b.chainID),
		Nonce:                b.nonce,
		MaxPriorityFeePerGas: bigOrZero(b.maxPriorityFeePerGas),
		MaxFeePerGas:         bigOrZero(b.maxFeePerGas),
		GasLimit:             b.gasLimit,
		To:                   b.to,
		Value:                bigOrZero(b.value),
		Data:                 b.data,
		// AccessList: nil — encodes as the RLP empty list 0xC0.
		YParity: sig[64],
		R:       new(big.Int).SetBytes(sig[:32]),
		S:       new(big.Int).SetBytes(sig[32:64]),
	})
	if err != nil {
		return nil, fmt.Errorf("EthBuilder: RLP encode failed: %w", err)
	}

	// EIP-2718: prepend the transaction type byte.
	return append([]byte{TypeEthTyped1559}, bodyRLP...), nil
}

// cloneBigInt returns a copy of v, or nil if v is nil.
// Used by setters to take ownership of the caller's value at set time, so that
// subsequent mutations of the caller's *big.Int do not affect the builder.
func cloneBigInt(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

// bigOrZero returns v if non-nil, or a freshly allocated zero *big.Int.
// Setters already own their copies (via cloneBigInt), so no additional copy is
// needed here — this only guards against nil fields left unset on the builder.
func bigOrZero(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return v
}
