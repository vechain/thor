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
	txType               Type
	chainTag             byte
	chainID              uint64 // EIP-155 chain id, used by TypeEthDynamicFee only
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

func NewBuilder(txType Type) *Builder {
	return &Builder{txType: txType}
}

// ChainTag set chain tag.
func (b *Builder) ChainTag(tag byte) *Builder {
	b.chainTag = tag
	return b
}

// ChainID sets the EIP-155 chain id used by TypeEthDynamicFee transactions.
// Ignored for VeChain-native types (which use ChainTag).
func (b *Builder) ChainID(id uint64) *Builder {
	b.chainID = id
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

// Build builds a tx object.
func (b *Builder) Build() *Transaction {
	switch b.txType {
	case TypeLegacy:
		return &Transaction{
			body: &legacyTransaction{
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
	case TypeEthDynamicFee:
		// 0x02 is single-clause at the wire level; pull (to, value, data) from
		// the first clause. Any extra clauses are a programming error since the
		// envelope can't represent them.
		if len(b.clauses) != 1 {
			panic("tx: TypeEthDynamicFee requires exactly one clause")
		}
		c := b.clauses[0]
		var to *thor.Address
		if dst := c.To(); dst != nil {
			cpy := *dst
			to = &cpy
		}
		value := new(big.Int)
		if v := c.Value(); v != nil {
			value.Set(v)
		}
		maxFee := new(big.Int)
		if b.maxFeePerGas != nil {
			maxFee.Set(b.maxFeePerGas)
		}
		maxPriority := new(big.Int)
		if b.maxPriorityFeePerGas != nil {
			maxPriority.Set(b.maxPriorityFeePerGas)
		}
		return &Transaction{
			body: &ethDynamicFeeTransaction{
				ChainID:              new(big.Int).SetUint64(b.chainID),
				Nonce:                b.nonce,
				MaxPriorityFeePerGas: maxPriority,
				MaxFeePerGas:         maxFee,
				GasLimit:             b.gas,
				To:                   to,
				Value:                value,
				Data:                 append([]byte(nil), c.Data()...),
				YParity:              0,
				R:                    new(big.Int),
				S:                    new(big.Int),
			},
		}
	default:
		return &Transaction{
			body: &dynamicFeeTransaction{
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
	}
}
