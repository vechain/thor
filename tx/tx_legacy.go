// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
)

type LegacyTransaction struct {
	ChainTag     byte
	BlockRef     uint64
	Expiration   uint32
	Clauses      []*Clause
	GasPriceCoef uint8
	Gas          uint64
	DependsOn    *thor.Bytes32 `rlp:"nil"`
	Nonce        uint64
	Reserved     reserved
	Signature    []byte
}

func (t *LegacyTransaction) txType() byte {
	return LegacyTxType
}

func (t *LegacyTransaction) copy() TxData {
	cpy := &LegacyTransaction{
		ChainTag:     t.ChainTag,
		BlockRef:     t.BlockRef,
		Expiration:   t.Expiration,
		Clauses:      make([]*Clause, len(t.Clauses)),
		GasPriceCoef: t.GasPriceCoef,
		Gas:          t.Gas,
		DependsOn:    t.DependsOn,
		Nonce:        t.Nonce,
		Reserved:     t.Reserved,
		Signature:    t.Signature,
	}
	copy(cpy.Clauses, t.Clauses)
	return cpy
}

func (t *LegacyTransaction) chainTag() byte {
	return t.ChainTag
}

func (t *LegacyTransaction) blockRef() uint64 {
	return t.BlockRef
}

func (t *LegacyTransaction) expiration() uint32 {
	return t.Expiration
}

func (t *LegacyTransaction) clauses() []*Clause {
	return t.Clauses
}

func (t *LegacyTransaction) gas() uint64 {
	return t.Gas
}

func (t *LegacyTransaction) gasPriceCoef() uint8 {
	return t.GasPriceCoef
}

func (t *LegacyTransaction) maxFeePerGas() *big.Int {
	// For legacy transactions, maxFeePerGas is determined by GasPriceCoef
	return new(big.Int).SetUint64(uint64(t.GasPriceCoef))
}

func (t *LegacyTransaction) maxPriorityFeePerGas() *big.Int {
	// For legacy transactions, maxPriorityFeePerGas is determined by GasPriceCoef
	return new(big.Int).SetUint64(uint64(t.GasPriceCoef))
}

func (t *LegacyTransaction) dependsOn() *thor.Bytes32 {
	return t.DependsOn
}

func (t *LegacyTransaction) nonce() uint64 {
	return t.Nonce
}

func (t *LegacyTransaction) reserved() reserved {
	return t.Reserved
}

func (t *LegacyTransaction) signature() []byte {
	return t.Signature
}

func (t *LegacyTransaction) setSignature(sig []byte) {
	t.Signature = sig
}

func (t *LegacyTransaction) encode(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		t.ChainTag,
		t.BlockRef,
		t.Expiration,
		t.Clauses,
		t.GasPriceCoef,
		t.Gas,
		t.DependsOn,
		t.Nonce,
		&t.Reserved,
	})
}
