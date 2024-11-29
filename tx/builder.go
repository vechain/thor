// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"encoding/binary"
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// Builder to make it easy to build transaction.
type Builder struct {
	legacyTx     LegacyTransaction
	dynamicFeeTx DynamicFeeTransaction
}

// ChainTag set chain tag.
func (b *Builder) ChainTag(tag byte) *Builder {
	b.legacyTx.ChainTag = tag
	b.dynamicFeeTx.ChainTag = tag
	return b
}

// Clause add a clause.
func (b *Builder) Clause(c *Clause) *Builder {
	b.legacyTx.Clauses = append(b.legacyTx.Clauses, c)
	b.dynamicFeeTx.Clauses = append(b.dynamicFeeTx.Clauses, c)
	return b
}

// GasPriceCoef set gas price coef.
func (b *Builder) GasPriceCoef(coef uint8) *Builder {
	b.legacyTx.GasPriceCoef = coef
	return b
}

// Gas set gas provision for tx.
func (b *Builder) Gas(gas uint64) *Builder {
	b.legacyTx.Gas = gas
	b.dynamicFeeTx.Gas = gas
	return b
}

// MaxFeePerGas set max fee per gas.
func (b *Builder) MaxFeePerGas(maxFeePerGas *big.Int) *Builder {
	b.dynamicFeeTx.MaxFeePerGas = maxFeePerGas
	return b
}

// MaxPriorityFeePerGas set max priority fee per gas.
func (b *Builder) MaxPriorityFeePerGas(maxPriorityFeePerGas *big.Int) *Builder {
	b.dynamicFeeTx.MaxPriorityFeePerGas = maxPriorityFeePerGas
	return b
}

// BlockRef set block reference.
func (b *Builder) BlockRef(br BlockRef) *Builder {
	b.legacyTx.BlockRef = binary.BigEndian.Uint64(br[:])
	b.dynamicFeeTx.BlockRef = binary.BigEndian.Uint64(br[:])
	return b
}

// Expiration set expiration.
func (b *Builder) Expiration(exp uint32) *Builder {
	b.legacyTx.Expiration = exp
	b.dynamicFeeTx.Expiration = exp
	return b
}

// Nonce set nonce.
func (b *Builder) Nonce(nonce uint64) *Builder {
	b.legacyTx.Nonce = nonce
	b.dynamicFeeTx.Nonce = nonce
	return b
}

// DependsOn set depended tx.
func (b *Builder) DependsOn(txID *thor.Bytes32) *Builder {
	if txID == nil {
		b.legacyTx.DependsOn = nil
		b.dynamicFeeTx.DependsOn = nil
	} else {
		cpy := *txID
		b.legacyTx.DependsOn = &cpy
		b.dynamicFeeTx.DependsOn = &cpy
	}
	return b
}

// Features set features.
func (b *Builder) Features(feat Features) *Builder {
	b.legacyTx.Reserved.Features = feat
	b.dynamicFeeTx.Reserved.Features = feat
	return b
}

// BuildLegacy builds legacy tx object.
func (b *Builder) BuildLegacy() *Transaction {
	tx := Transaction{body: &b.legacyTx}
	return &tx
}

// BuildDynamicFee builds dynamic fee tx object.
func (b *Builder) BuildDynamicFee() *Transaction {
	tx := Transaction{body: &b.dynamicFeeTx}
	return &tx
}
