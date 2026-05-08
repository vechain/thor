// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

// AccessListEntry is a single entry in an EIP-2930 / EIP-1559 access list.
// Non-empty access lists are rejected at runtime.ResolveTransaction until
// EIP-2930 warm/cold gas accounting is implemented.
type AccessListEntry struct {
	Address     thor.Address
	StorageKeys []thor.Bytes32
}

// ethDynamicFeeEmptyReserved is a package-level zero-value sentinel returned by reserved().
// Avoids a heap allocation on every call since ethDynamicFeeTransaction never sets reserved flags.
var ethDynamicFeeEmptyReserved reserved

// ethDynamicFeeTransaction implements txData for EIP-1559 typed Ethereum transactions
// (type byte 0x02; wire format: 0x02 || RLP([chainId, nonce, ...])).
//
// VeChain-specific fields that Ethereum transactions do not carry are stubbed:
//
//	blockRef   = 0          → blockRef.Number()=0 ≤ any block; all schedule checks pass.
//	expiration = MaxUint32  → IsExpired (blockNum > 0 + MaxUint32) is never true.
//	chainTag   = 0          → Ethereum replay protection uses chainID; chain tag validation
//	                          is bypassed in txpool/packer/consensus for Ethereum tx types.
//	dependsOn  = nil        → no dependency on another transaction.
//	reserved   = {}         → no feature flags; not delegated; 65-byte signature expected.
type ethDynamicFeeTransaction struct {
	ChainID              *big.Int
	Nonce                uint64
	MaxPriorityFeePerGas *big.Int
	MaxFeePerGas         *big.Int
	GasLimit             uint64
	To                   *thor.Address `rlp:"nil"` // nil = contract creation
	Value                *big.Int
	Data                 []byte
	AccessList           []AccessListEntry
	YParity              uint8
	R                    *big.Int
	S                    *big.Int
}

func (t *ethDynamicFeeTransaction) txType() byte             { return TypeEthDynamicFee }
func (t *ethDynamicFeeTransaction) chainTag() byte           { return 0 } // Ethereum txs use chainID; chain tag validation is bypassed
func (t *ethDynamicFeeTransaction) blockRef() uint64         { return 0 }
func (t *ethDynamicFeeTransaction) expiration() uint32       { return math.MaxUint32 }
func (t *ethDynamicFeeTransaction) gas() uint64              { return t.GasLimit }
func (t *ethDynamicFeeTransaction) dependsOn() *thor.Bytes32 { return nil }
func (t *ethDynamicFeeTransaction) nonce() uint64            { return t.Nonce }
func (t *ethDynamicFeeTransaction) reserved() *reserved      { return &ethDynamicFeeEmptyReserved }

func (t *ethDynamicFeeTransaction) clauses() []*Clause {
	return []*Clause{NewClause(t.To).WithValue(t.Value).WithData(t.Data)}
}

func (t *ethDynamicFeeTransaction) maxFeePerGas() *big.Int {
	return new(big.Int).Set(t.MaxFeePerGas)
}

func (t *ethDynamicFeeTransaction) maxPriorityFeePerGas() *big.Int {
	return new(big.Int).Set(t.MaxPriorityFeePerGas)
}

// signingFields returns the 9 EIP-1559 signing-preimage fields. RLP encodes a
// nil *thor.Address (contract creation) as the canonical empty string 0x80,
// matching the wire format produced by go-ethereum's DynamicFeeTx.sigHash.
func (t *ethDynamicFeeTransaction) signingFields() []any {
	return []any{
		t.ChainID,
		t.Nonce,
		t.MaxPriorityFeePerGas,
		t.MaxFeePerGas,
		t.GasLimit,
		t.To,
		t.Value,
		t.Data,
		t.AccessList,
	}
}

func (t *ethDynamicFeeTransaction) signature() []byte {
	sig := make([]byte, 65)
	t.R.FillBytes(sig[0:32])
	t.S.FillBytes(sig[32:64])
	sig[64] = t.YParity
	return sig
}

// setSignature writes R/S/yParity from the 65-byte input. Called only from
// Transaction.WithSignature on a freshly-copied body; the parent *Transaction
// gets a fresh cache, so ID/Hash/Origin are recomputed lazily.
func (t *ethDynamicFeeTransaction) setSignature(sig []byte) {
	t.R = new(big.Int).SetBytes(sig[:32])
	t.S = new(big.Int).SetBytes(sig[32:64])
	t.YParity = sig[64]
}

func (t *ethDynamicFeeTransaction) copy() txData {
	var to *thor.Address
	if t.To != nil {
		cpy := *t.To
		to = &cpy
	}
	cpy := &ethDynamicFeeTransaction{
		ChainID:              new(big.Int).Set(t.ChainID),
		Nonce:                t.Nonce,
		MaxPriorityFeePerGas: new(big.Int).Set(t.MaxPriorityFeePerGas),
		MaxFeePerGas:         new(big.Int).Set(t.MaxFeePerGas),
		GasLimit:             t.GasLimit,
		To:                   to,
		Value:                new(big.Int).Set(t.Value),
		Data:                 bytes.Clone(t.Data),
		YParity:              t.YParity,
		R:                    new(big.Int).Set(t.R),
		S:                    new(big.Int).Set(t.S),
	}
	if len(t.AccessList) > 0 {
		cpy.AccessList = make([]AccessListEntry, len(t.AccessList))
		copy(cpy.AccessList, t.AccessList)
	}
	return cpy
}

// encode writes the rlpBody part (without the 0x02 type byte). Transaction.encodeTyped
// prepends the type byte, producing the standard EIP-1559 wire format: 0x02 || rlpBody.
func (t *ethDynamicFeeTransaction) encode(w *bytes.Buffer) error {
	return rlp.Encode(w, t)
}

// decode parses the rlpBody (without the leading 0x02 byte) from the block body.
// Only structural parsing happens here. Semantic validation belongs to the caller —
// txpool.validateTxBasics (chain ID, low-S), runtime.ResolveTransaction (fee/access
// list), consensus.validateBlockBody (chain ID, type gating).
func (t *ethDynamicFeeTransaction) decode(input []byte) error {
	return rlp.DecodeBytes(input, t)
}
