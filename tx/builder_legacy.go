// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"encoding/binary"

	"github.com/vechain/thor/v2/thor"
)

// LegacyBuilder to make it easy to build transaction.
type LegacyBuilder struct {
	legacyTx LegacyTransaction
}

// ChainTag set chain tag.
func (b *LegacyBuilder) ChainTag(tag byte) *LegacyBuilder {
	b.legacyTx.ChainTag = tag
	return b
}

// Clause add a clause.
func (b *LegacyBuilder) Clause(c *Clause) *LegacyBuilder {
	b.legacyTx.Clauses = append(b.legacyTx.Clauses, c)
	return b
}

// GasPriceCoef set gas price coef.
func (b *LegacyBuilder) GasPriceCoef(coef uint8) *LegacyBuilder {
	b.legacyTx.GasPriceCoef = coef
	return b
}

// Gas set gas provision for tx.
func (b *LegacyBuilder) Gas(gas uint64) *LegacyBuilder {
	b.legacyTx.Gas = gas
	return b
}

// BlockRef set block reference.
func (b *LegacyBuilder) BlockRef(br BlockRef) *LegacyBuilder {
	b.legacyTx.BlockRef = binary.BigEndian.Uint64(br[:])
	return b
}

// Expiration set expiration.
func (b *LegacyBuilder) Expiration(exp uint32) *LegacyBuilder {
	b.legacyTx.Expiration = exp
	return b
}

// Nonce set nonce.
func (b *LegacyBuilder) Nonce(nonce uint64) *LegacyBuilder {
	b.legacyTx.Nonce = nonce
	return b
}

// DependsOn set depended tx.
func (b *LegacyBuilder) DependsOn(txID *thor.Bytes32) *LegacyBuilder {
	if txID == nil {
		b.legacyTx.DependsOn = nil
	} else {
		cpy := *txID
		b.legacyTx.DependsOn = &cpy
	}
	return b
}

// Features set features.
func (b *LegacyBuilder) Features(feat Features) *LegacyBuilder {
	b.legacyTx.Reserved.Features = feat
	return b
}

// BuildLegacy builds legacy tx object.
func (b *LegacyBuilder) Build() *Transaction {
	tx := Transaction{body: &b.legacyTx}
	return &tx
}
