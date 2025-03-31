// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
)

type legacyTransaction struct {
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

func (t *legacyTransaction) copy() txData {
	cpy := &legacyTransaction{
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

func (t *legacyTransaction) txType() byte             { return TypeLegacy }
func (t *legacyTransaction) chainTag() byte           { return t.ChainTag }
func (t *legacyTransaction) blockRef() uint64         { return t.BlockRef }
func (t *legacyTransaction) expiration() uint32       { return t.Expiration }
func (t *legacyTransaction) clauses() []*Clause       { return t.Clauses }
func (t *legacyTransaction) gas() uint64              { return t.Gas }
func (t *legacyTransaction) gasPriceCoef() uint8      { return t.GasPriceCoef }
func (t *legacyTransaction) dependsOn() *thor.Bytes32 { return t.DependsOn }
func (t *legacyTransaction) nonce() uint64            { return t.Nonce }
func (t *legacyTransaction) reserved() *reserved      { return &t.Reserved }
func (t *legacyTransaction) signature() []byte        { return t.Signature }

func (t *legacyTransaction) setSignature(sig []byte) {
	t.Signature = sig
}

func (t *legacyTransaction) signingFields() []any {
	return []any{
		t.ChainTag,
		t.BlockRef,
		t.Expiration,
		t.Clauses,
		t.GasPriceCoef,
		t.Gas,
		t.DependsOn,
		t.Nonce,
		&t.Reserved,
	}
}

func (t *legacyTransaction) evaluateWork(origin thor.Address) func(nonce uint64) *big.Int {
	hashWithoutNonce := thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, []any{
			t.ChainTag,
			t.BlockRef,
			t.Expiration,
			t.Clauses,
			t.GasPriceCoef,
			t.Gas,
			t.DependsOn,
			&t.Reserved,
			origin,
		})
	})

	return func(nonce uint64) *big.Int {
		var nonceBytes [8]byte
		binary.BigEndian.PutUint64(nonceBytes[:], nonce)
		hash := thor.Blake2b(hashWithoutNonce[:], nonceBytes[:])
		r := new(big.Int).SetBytes(hash[:])
		return r.Div(math.MaxBig256, r)
	}
}

// Below are the methods that are not compatible with legacy transaction
func (t *legacyTransaction) maxFeePerGas() *big.Int         { return common.Big0 } // Return default value as they are not meant to be used anywhere else
func (t *legacyTransaction) maxPriorityFeePerGas() *big.Int { return common.Big0 } // Return default value as they are not meant to be used anywhere else

func (t *legacyTransaction) encode(*bytes.Buffer) error {
	panic("encode called on LegacyTx")
}

func (t *legacyTransaction) decode([]byte) error {
	panic("decode called on LegacyTx")
}
