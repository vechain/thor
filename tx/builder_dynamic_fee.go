// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"encoding/binary"
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// DynFeeBuilder to make it easy to build transaction.
type DynFeeBuilder struct {
	dynamicFeeTx DynamicFeeTransaction
}

// ChainTag set chain tag.
func (b *DynFeeBuilder) ChainTag(tag byte) *DynFeeBuilder {
	b.dynamicFeeTx.ChainTag = tag
	return b
}

// Clause add a clause.
func (b *DynFeeBuilder) Clause(c *Clause) *DynFeeBuilder {
	b.dynamicFeeTx.Clauses = append(b.dynamicFeeTx.Clauses, c)
	return b
}

// Gas set gas provision for tx.
func (b *DynFeeBuilder) Gas(gas uint64) *DynFeeBuilder {
	b.dynamicFeeTx.Gas = gas
	return b
}

// MaxFeePerGas set max fee per gas.
func (b *DynFeeBuilder) MaxFeePerGas(maxFeePerGas *big.Int) *DynFeeBuilder {
	b.dynamicFeeTx.MaxFeePerGas = maxFeePerGas
	return b
}

// MaxPriorityFeePerGas set max priority fee per gas.
func (b *DynFeeBuilder) MaxPriorityFeePerGas(maxPriorityFeePerGas *big.Int) *DynFeeBuilder {
	b.dynamicFeeTx.MaxPriorityFeePerGas = maxPriorityFeePerGas
	return b
}

// BlockRef set block reference.
func (b *DynFeeBuilder) BlockRef(br BlockRef) *DynFeeBuilder {
	b.dynamicFeeTx.BlockRef = binary.BigEndian.Uint64(br[:])
	return b
}

// Expiration set expiration.
func (b *DynFeeBuilder) Expiration(exp uint32) *DynFeeBuilder {
	b.dynamicFeeTx.Expiration = exp
	return b
}

// Nonce set nonce.
func (b *DynFeeBuilder) Nonce(nonce uint64) *DynFeeBuilder {
	b.dynamicFeeTx.Nonce = nonce
	return b
}

// DependsOn set depended tx.
func (b *DynFeeBuilder) DependsOn(txID *thor.Bytes32) *DynFeeBuilder {
	if txID == nil {
		b.dynamicFeeTx.DependsOn = nil
	} else {
		cpy := *txID
		b.dynamicFeeTx.DependsOn = &cpy
	}
	return b
}

// Features set features.
func (b *DynFeeBuilder) Features(feat Features) *DynFeeBuilder {
	b.dynamicFeeTx.Reserved.Features = feat
	return b
}

// BuildDynamicFee builds dynamic fee tx object.
func (b *DynFeeBuilder) Build() *Transaction {
	tx := Transaction{body: &b.dynamicFeeTx}
	return &tx
}
