// Copyright (c) 2024 The VeChainThor developers

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
	txType               int
	chainTag             byte
	clauses              []*Clause
	gasPriceCoef         uint8
	maxFeePerGas         *big.Int
	maxPriorityFeePerGas *big.Int
	gas                  uint64
	blockRef             uint64
	expiration           uint32
	nonce                uint64
	dependsOn            *thor.Bytes32
	reserved             reserved
}

func NewTxBuilder(txType int) *Builder {
	return &Builder{txType: txType}
}

// ChainTag set chain tag.
func (b *Builder) ChainTag(tag byte) *Builder {
	b.chainTag = tag
	return b
}

// Clause add a clause.
func (b *Builder) Clause(c *Clause) *Builder {
	b.clauses = append(b.clauses, c)
	return b
}

func (b *Builder) Clauses(clauses []*Clause) *Builder {
	for _, c := range clauses {
		b.Clause(c)
	}
	return b
}

// GasPriceCoef set gas price coef.
func (b *Builder) GasPriceCoef(coef uint8) *Builder {
	b.gasPriceCoef = coef
	return b
}

// MaxFeePerGas set max fee per gas.
func (b *Builder) MaxFeePerGas(maxFeePerGas *big.Int) *Builder {
	b.maxFeePerGas = maxFeePerGas
	return b
}

// MaxPriorityFeePerGas set max priority fee per gas.
func (b *Builder) MaxPriorityFeePerGas(maxPriorityFeePerGas *big.Int) *Builder {
	b.maxPriorityFeePerGas = maxPriorityFeePerGas
	return b
}

// Gas set gas provision for tx.
func (b *Builder) Gas(gas uint64) *Builder {
	b.gas = gas
	return b
}

// BlockRef set block reference.
func (b *Builder) BlockRef(br BlockRef) *Builder {
	b.blockRef = binary.BigEndian.Uint64(br[:])
	return b
}

// Expiration set expiration.
func (b *Builder) Expiration(exp uint32) *Builder {
	b.expiration = exp
	return b
}

// Nonce set nonce.
func (b *Builder) Nonce(nonce uint64) *Builder {
	b.nonce = nonce
	return b
}

// DependsOn set depended tx.
func (b *Builder) DependsOn(txID *thor.Bytes32) *Builder {
	if txID == nil {
		b.dependsOn = nil
	} else {
		cpy := *txID
		b.dependsOn = &cpy
	}
	return b
}

// Features set features.
func (b *Builder) Features(feat Features) *Builder {
	b.reserved.Features = feat
	return b
}

// BuildLegacy builds legacy tx object.
func (b *Builder) Build() (*Transaction, error) {
	var tx *Transaction
	switch b.txType {
	case LegacyTxType:
		tx = &Transaction{
			body: &LegacyTransaction{
				ChainTag:     b.chainTag,
				Clauses:      b.clauses,
				GasPriceCoef: b.gasPriceCoef,
				Gas:          b.gas,
				BlockRef:     b.blockRef,
				Expiration:   b.expiration,
				Nonce:        b.nonce,
				DependsOn:    b.dependsOn,
				Reserved:     b.reserved,
			},
		}
	case DynamicFeeTxType:
		tx = &Transaction{
			body: &DynamicFeeTransaction{
				ChainTag:             b.chainTag,
				Clauses:              b.clauses,
				MaxFeePerGas:         b.maxFeePerGas,
				MaxPriorityFeePerGas: b.maxPriorityFeePerGas,
				Gas:                  b.gas,
				BlockRef:             b.blockRef,
				Expiration:           b.expiration,
				Nonce:                b.nonce,
				DependsOn:            b.dependsOn,
				Reserved:             b.reserved,
			},
		}
	default:
		return nil, ErrTxTypeNotSupported
	}
	return tx, nil
}
