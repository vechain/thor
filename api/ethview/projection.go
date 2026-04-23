// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethview

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// TxMeta carries the chain-side metadata the view layer needs to render a
// TransactionObject. For a pending tx the caller sets BlockID, BlockNumber and
// Index to the zero / nil values; Origin is always required (pre-recovered).
type TxMeta struct {
	BlockID     *thor.Bytes32 // nil for pending
	BlockNumber *uint32       // nil for pending
	Index       uint32        // position in the canonical block; zero for pending
	Origin      thor.Address  // signer / from
	Delegator   *thor.Address // VIP-191 delegator, nil if not delegated

	// EffectiveGasPrice is the per-gas price paid on chain (from the receipt).
	// Nil for pending txs. Required for 0x51 / 0x02 mined projections; ignored
	// for 0x00 (which derives gas price from gasPriceCoef).
	EffectiveGasPrice *big.Int
}

// TransactionObject is the eth-shaped tx view. Standard eth fields are always
// present; VeChainTx extension fields are populated only when the native type
// is 0x00 or 0x51 (single clause). The struct is shared by all three tx
// types — unused fields are left zero / nil and omitted on the wire via the
// omitempty tag.
type TransactionObject struct {
	// Standard eth fields.
	Hash                 thor.Bytes32    `json:"hash"`
	Type                 hexutil.Uint64  `json:"type"`
	ChainID              *hexutil.Big    `json:"chainId,omitempty"`
	Nonce                hexutil.Uint64  `json:"nonce"`
	BlockHash            *thor.Bytes32   `json:"blockHash"`
	BlockNumber          *hexutil.Uint64 `json:"blockNumber"`
	TransactionIndex     *hexutil.Uint64 `json:"transactionIndex"`
	From                 thor.Address    `json:"from"`
	To                   *thor.Address   `json:"to"`
	Value                *hexutil.Big    `json:"value"`
	Gas                  hexutil.Uint64  `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas,omitempty"`
	AccessList           *[]accessEntry  `json:"accessList,omitempty"`
	Input                hexutil.Bytes   `json:"input"`
	V                    *hexutil.Big    `json:"v"`
	R                    *hexutil.Big    `json:"r"`
	S                    *hexutil.Big    `json:"s"`

	// VeChainTx extension fields (0x00 / 0x51 single-clause only).
	ChainTag     *hexutil.Uint64       `json:"chainTag,omitempty"`
	BlockRef     *hexutil.Bytes        `json:"blockRef,omitempty"`
	Expiration   *hexutil.Uint64       `json:"expiration,omitempty"`
	Clauses      []nativeClause        `json:"clauses,omitempty"`
	DependsOn    *thor.Bytes32         `json:"dependsOn,omitempty"`
	Reserved     *nativeReservedStruct `json:"reserved,omitempty"`
	Delegator    *thor.Address         `json:"delegator,omitempty"`
	GasPriceCoef *hexutil.Uint64       `json:"gasPriceCoef,omitempty"`
}

// isRepresentable returns true when trx has a faithful eth-shape projection.
// 0x02 is always representable; 0x00 / 0x51 only when they carry exactly one
// clause.
func isRepresentable(trx *tx.Transaction) bool {
	switch trx.Type() {
	case tx.TypeEthDynamicFee:
		return true
	case tx.TypeLegacy, tx.TypeDynamicFee:
		return len(trx.Clauses()) == 1
	default:
		return false
	}
}

// ProjectTx maps a native *tx.Transaction plus caller-supplied metadata into a
// TransactionObject. Multi-clause legacy / dynamic-fee txs yield
// ErrNotRepresentable — callers translate that sentinel to the
// "tx_not_representable" JSON-RPC reason.
func ProjectTx(trx *tx.Transaction, meta TxMeta) (*TransactionObject, error) {
	switch trx.Type() {
	case tx.TypeEthDynamicFee:
		return projectEthDynamicFee(trx, meta), nil
	case tx.TypeLegacy, tx.TypeDynamicFee:
		if !isRepresentable(trx) {
			return nil, ErrNotRepresentable
		}
		return projectVeChainTx(trx, meta), nil
	default:
		return nil, ErrNotRepresentable
	}
}

// projectEthDynamicFee renders a 0x02 tx. No extension fields.
func projectEthDynamicFee(trx *tx.Transaction, meta TxMeta) *TransactionObject {
	v, r, s := splitSignatureEth(trx)

	obj := &TransactionObject{
		Hash:                 trx.CanonicalTxID(),
		Type:                 hexutil.Uint64(tx.TypeEthDynamicFee),
		ChainID:              (*hexutil.Big)(trx.ChainID()),
		Nonce:                hexutil.Uint64(trx.Nonce()),
		From:                 meta.Origin,
		To:                   firstClauseTo(trx),
		Value:                (*hexutil.Big)(firstClauseValue(trx)),
		Gas:                  hexutil.Uint64(trx.Gas()),
		MaxFeePerGas:         (*hexutil.Big)(trx.MaxFeePerGas()),
		MaxPriorityFeePerGas: (*hexutil.Big)(trx.MaxPriorityFeePerGas()),
		AccessList:           projectAccessList(trx.AccessList()),
		Input:                firstClauseData(trx),
		V:                    (*hexutil.Big)(v),
		R:                    (*hexutil.Big)(r),
		S:                    (*hexutil.Big)(s),
	}

	// gasPrice: effective (mined) or fee cap (pending).
	if meta.BlockID == nil {
		obj.GasPrice = (*hexutil.Big)(new(big.Int).Set(trx.MaxFeePerGas()))
	} else if meta.EffectiveGasPrice != nil {
		obj.GasPrice = (*hexutil.Big)(new(big.Int).Set(meta.EffectiveGasPrice))
	} else {
		obj.GasPrice = (*hexutil.Big)(new(big.Int))
	}

	// Mined vs pending location.
	if meta.BlockID != nil {
		idx := hexutil.Uint64(meta.Index)
		obj.BlockHash = meta.BlockID
		if meta.BlockNumber != nil {
			bn := hexutil.Uint64(*meta.BlockNumber)
			obj.BlockNumber = &bn
		}
		obj.TransactionIndex = &idx
	}

	return obj
}

// --- helpers -------------------------------------------------------------

// firstClauseTo returns the single-clause `to` address. 0x02 always has
// exactly one clause by construction (set at decode time).
func firstClauseTo(trx *tx.Transaction) *thor.Address {
	cs := trx.Clauses()
	if len(cs) == 0 {
		return nil
	}
	return cs[0].To()
}

func firstClauseValue(trx *tx.Transaction) *big.Int {
	cs := trx.Clauses()
	if len(cs) == 0 {
		return new(big.Int)
	}
	return cs[0].Value()
}

func firstClauseData(trx *tx.Transaction) hexutil.Bytes {
	cs := trx.Clauses()
	if len(cs) == 0 {
		return nil
	}
	return cs[0].Data()
}

// splitSignatureEth returns the (V, R, S) triplet for 0x02 / 0x51 signatures.
// The native 65-byte signature layout is R (32) || S (32) || V (1); V is 0/1.
func splitSignatureEth(trx *tx.Transaction) (*big.Int, *big.Int, *big.Int) {
	sig := trx.Signature()
	if len(sig) != 65 {
		return new(big.Int), new(big.Int), new(big.Int)
	}
	r := new(big.Int).SetBytes(sig[0:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetUint64(uint64(sig[64]))
	return v, r, s
}

// projectAccessList renders an eth-shape access list. 0x02 always has an
// empty list today (non-empty is rejected at ingestion). Returning a pointer
// to an empty slice so json emits `"accessList":[]` rather than omitting.
func projectAccessList(al tx.AccessList) *[]accessEntry {
	out := make([]accessEntry, 0, len(al))
	for _, t := range al {
		keys := make([]thor.Bytes32, len(t.StorageKeys))
		copy(keys, t.StorageKeys)
		out = append(out, accessEntry{Address: t.Address, StorageKeys: keys})
	}
	return &out
}

// projectVeChainTx renders a single-clause 0x00 or 0x51 tx. Callers must have
// already rejected multi-clause txs; this function assumes len(clauses)==1.
// VeChainTx-specific fields (chainTag, blockRef, expiration, clauses,
// dependsOn, reserved, delegator, gasPriceCoef) are populated; eth standard
// fields are copied from the first (and only) clause.
func projectVeChainTx(trx *tx.Transaction, meta TxMeta) *TransactionObject {
	cs := trx.Clauses()
	first := cs[0]
	v, r, s := splitSignatureVeChain(trx)

	chainTag := hexutil.Uint64(trx.ChainTag())
	blockRefBytes := trx.BlockRef()
	blockRef := hexutil.Bytes(blockRefBytes[:])
	expiration := hexutil.Uint64(trx.Expiration())
	gpc := hexutil.Uint64(trx.GasPriceCoef())

	// Clause mirror array — single-clause by precondition.
	clauses := []nativeClause{{
		To:    first.To(),
		Value: (*hexutil.Big)(first.Value()),
		Data:  first.Data(),
	}}

	obj := &TransactionObject{
		Hash:       trx.CanonicalTxID(),
		Type:       hexutil.Uint64(trx.Type()),
		Nonce:      hexutil.Uint64(trx.Nonce()),
		From:       meta.Origin,
		To:         first.To(),
		Value:      (*hexutil.Big)(first.Value()),
		Gas:        hexutil.Uint64(trx.Gas()),
		Input:      first.Data(),
		V:          (*hexutil.Big)(v),
		R:          (*hexutil.Big)(r),
		S:          (*hexutil.Big)(s),
		ChainTag:   &chainTag,
		BlockRef:   &blockRef,
		Expiration: &expiration,
		Clauses:    clauses,
		DependsOn:  trx.DependsOn(),
		Delegator:  meta.Delegator,
	}

	// Reserved trailer is only meaningful when Features != 0 (today that means
	// VIP-191 delegation). Emit whenever non-zero.
	if feat := trx.Features(); feat != 0 {
		obj.Reserved = &nativeReservedStruct{Features: hexutil.Uint64(feat)}
	}

	// gasPriceCoef is 0x00-only; skip serializing for 0x51.
	if trx.Type() == tx.TypeLegacy {
		obj.GasPriceCoef = &gpc
	}

	// 0x51 carries its own fee caps — surface them like 0x02.
	if trx.Type() == tx.TypeDynamicFee {
		obj.MaxFeePerGas = (*hexutil.Big)(trx.MaxFeePerGas())
		obj.MaxPriorityFeePerGas = (*hexutil.Big)(trx.MaxPriorityFeePerGas())
	}

	// gasPrice derivation per spec §6.4. Caller is responsible for supplying
	// meta.EffectiveGasPrice:
	//   0x00 mined or pending: bgp × (255 + gasPriceCoef) / 255.
	//   0x51 mined: receipt effectiveGasPrice; pending: maxFeePerGas.
	switch {
	case meta.EffectiveGasPrice != nil:
		obj.GasPrice = (*hexutil.Big)(new(big.Int).Set(meta.EffectiveGasPrice))
	case trx.Type() == tx.TypeDynamicFee:
		obj.GasPrice = (*hexutil.Big)(new(big.Int).Set(trx.MaxFeePerGas()))
	default:
		obj.GasPrice = (*hexutil.Big)(new(big.Int))
	}

	// Block location (same rule as 0x02).
	if meta.BlockID != nil {
		idx := hexutil.Uint64(meta.Index)
		obj.BlockHash = meta.BlockID
		if meta.BlockNumber != nil {
			bn := hexutil.Uint64(*meta.BlockNumber)
			obj.BlockNumber = &bn
		}
		obj.TransactionIndex = &idx
	}

	return obj
}

// splitSignatureVeChain returns (V, R, S) from a VeChainTx signature. For
// non-delegated 0x00 / 0x51 the signature is a single 65-byte ECDSA blob;
// for delegated txs the first 65 bytes are the originator's sig and the
// remaining 65 are the delegator's. We only expose the originator triplet in
// the eth-shape (wallets and tools that care about the delegator read it via
// the dedicated `delegator` field).
func splitSignatureVeChain(trx *tx.Transaction) (*big.Int, *big.Int, *big.Int) {
	sig := trx.Signature()
	if len(sig) < 65 {
		return new(big.Int), new(big.Int), new(big.Int)
	}
	r := new(big.Int).SetBytes(sig[0:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetUint64(uint64(sig[64]))
	return v, r, s
}

