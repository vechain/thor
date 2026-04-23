// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

// AccessTuple is the element type of an EIP-2930 access list, laid out to
// match go-ethereum's types.AccessTuple so RLP encoding is bit-exact with
// what Ethereum wallets produce.
type AccessTuple struct {
	Address     thor.Address
	StorageKeys []thor.Bytes32
}

// AccessList is an EIP-2930 access list. VeChain currently rejects non-empty
// access lists at runtime (they encode/decode correctly for bit-compat with
// Ethereum tx hashes, but no runtime support exists yet).
type AccessList []AccessTuple

// ethDynamicFeeTransaction is the in-memory representation of an Ethereum
// EIP-1559 (type 0x02) transaction carried inside VeChainThor.
//
// Layout matches go-ethereum's DynamicFeeTx so that canonical encoding and
// keccak256 txhash agree with Ethereum wallets.
//
// Nonce semantics: the Nonce field is treated as a user-chosen random value,
// not a sequential account counter. Admission is NOT gated by
// tx.Nonce == state_account_nonce; de-duplication relies on CanonicalTxID
// (keccak256 of the signed RLP bytes) being unique per distinct signed tx.
// This matches VeChain's legacy (0x00) and dynamic-fee (0x51) nonce model.
//
// Implementing a true sequential account nonce plus Ethereum-style CREATE
// address derivation (keccak(rlp(caller, caller.Nonce))[12:]) is deferred;
// CREATE addresses for all tx types go through thor.CreateContractAddress
// in the current design.
type ethDynamicFeeTransaction struct {
	ChainID              *big.Int
	Nonce                uint64
	MaxPriorityFeePerGas *big.Int
	MaxFeePerGas         *big.Int
	Gas                  uint64
	To                   *thor.Address `rlp:"nil"` // nil means contract creation
	Value                *big.Int
	Data                 []byte
	AccessList           AccessList
	V, R, S              *big.Int // signature values; V is the y-parity (0 or 1)
}

func (t *ethDynamicFeeTransaction) copy() txData {
	cpy := &ethDynamicFeeTransaction{
		Nonce:                t.Nonce,
		Gas:                  t.Gas,
		Data:                 bytes.Clone(t.Data),
		AccessList:           append(AccessList(nil), t.AccessList...),
		ChainID:              new(big.Int),
		MaxPriorityFeePerGas: new(big.Int),
		MaxFeePerGas:         new(big.Int),
		Value:                new(big.Int),
		V:                    new(big.Int),
		R:                    new(big.Int),
		S:                    new(big.Int),
	}
	if t.To != nil {
		to := *t.To
		cpy.To = &to
	}
	if t.ChainID != nil {
		cpy.ChainID.Set(t.ChainID)
	}
	if t.MaxPriorityFeePerGas != nil {
		cpy.MaxPriorityFeePerGas.Set(t.MaxPriorityFeePerGas)
	}
	if t.MaxFeePerGas != nil {
		cpy.MaxFeePerGas.Set(t.MaxFeePerGas)
	}
	if t.Value != nil {
		cpy.Value.Set(t.Value)
	}
	if t.V != nil {
		cpy.V.Set(t.V)
	}
	if t.R != nil {
		cpy.R.Set(t.R)
	}
	if t.S != nil {
		cpy.S.Set(t.S)
	}
	return cpy
}

func (t *ethDynamicFeeTransaction) txType() byte       { return TypeEthDynamicFee }
func (t *ethDynamicFeeTransaction) chainTag() byte     { return 0 } // no ChainTag — ChainID plays that role
func (t *ethDynamicFeeTransaction) blockRef() uint64   { return 0 } // ETH tx has no block reference
func (t *ethDynamicFeeTransaction) expiration() uint32 { return 0 } // ETH tx never expires (handled in IsExpired)
func (t *ethDynamicFeeTransaction) gas() uint64        { return t.Gas }
func (t *ethDynamicFeeTransaction) dependsOn() *thor.Bytes32 {
	return nil
}

func (t *ethDynamicFeeTransaction) nonce() uint64       { return t.Nonce }
func (t *ethDynamicFeeTransaction) reserved() *reserved { return &reserved{} }

func (t *ethDynamicFeeTransaction) clauses() []*Clause {
	// Single synthetic clause derived from (to, value, data).
	var toCopy *thor.Address
	if t.To != nil {
		cpy := *t.To
		toCopy = &cpy
	}
	c := NewClause(toCopy)
	if t.Value != nil {
		c = c.WithValue(t.Value)
	}
	if len(t.Data) > 0 {
		c = c.WithData(t.Data)
	}
	return []*Clause{c}
}

func (t *ethDynamicFeeTransaction) maxFeePerGas() *big.Int {
	if t.MaxFeePerGas == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(t.MaxFeePerGas)
}

func (t *ethDynamicFeeTransaction) maxPriorityFeePerGas() *big.Int {
	if t.MaxPriorityFeePerGas == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(t.MaxPriorityFeePerGas)
}

// signature returns R(32) || S(32) || V(1) — 65 bytes. V is the y-parity (0 or 1).
// Returns nil if the tx is unsigned.
func (t *ethDynamicFeeTransaction) signature() []byte {
	if t.V == nil || t.R == nil || t.S == nil {
		return nil
	}
	if t.V.Sign() == 0 && t.R.Sign() == 0 && t.S.Sign() == 0 {
		return nil
	}
	sig := make([]byte, 65)
	t.R.FillBytes(sig[:32])
	t.S.FillBytes(sig[32:64])
	// V is 0 or 1 for EIP-1559.
	if t.V.Sign() != 0 {
		sig[64] = byte(t.V.Uint64())
	}
	return sig
}

func (t *ethDynamicFeeTransaction) setSignature(sig []byte) {
	if len(sig) != 65 {
		// Caller guarantees 65 bytes for 0x02; defensive default to zeros.
		t.R = new(big.Int)
		t.S = new(big.Int)
		t.V = new(big.Int)
		return
	}
	t.R = new(big.Int).SetBytes(sig[:32])
	t.S = new(big.Int).SetBytes(sig[32:64])
	t.V = new(big.Int).SetUint64(uint64(sig[64]))
}

// signingFields returns the fields used to compute SigningHash:
// [chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gas, to, value, data, accessList]
// The hash is Keccak256(0x02 || RLP(signingFields)).
func (t *ethDynamicFeeTransaction) signingFields() []any {
	return []any{
		t.ChainID,
		t.Nonce,
		t.MaxPriorityFeePerGas,
		t.MaxFeePerGas,
		t.Gas,
		t.To,
		t.Value,
		t.Data,
		t.AccessList,
	}
}

func (t *ethDynamicFeeTransaction) encode(b *bytes.Buffer) error {
	return rlp.Encode(b, t)
}

func (t *ethDynamicFeeTransaction) decode(input []byte) error {
	// ChainID / fee fields are validated at admission (txpool) and consensus;
	// decode only enforces RLP well-formedness so that bit-exact re-encoding is
	// possible for hash verification.
	return rlp.DecodeBytes(input, t)
}
