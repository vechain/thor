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

	// 0x02 (ETH EIP-1559) specific. Ignored for other tx types.
	chainID    *big.Int
	ethTo      *thor.Address
	ethValue   *big.Int
	ethData    []byte
	accessList AccessList
}

func NewBuilder(txType Type) *Builder {
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

// ChainID sets the Ethereum chainID used by type 0x02 transactions. Ignored
// for non-0x02 types.
func (b *Builder) ChainID(chainID *big.Int) *Builder {
	if chainID != nil {
		b.chainID = new(big.Int).Set(chainID)
	}
	return b
}

// EthTo sets the recipient of a type 0x02 transaction. nil means contract
// creation. Ignored for non-0x02 types.
func (b *Builder) EthTo(to *thor.Address) *Builder {
	if to != nil {
		cpy := *to
		b.ethTo = &cpy
	} else {
		b.ethTo = nil
	}
	return b
}

// EthValue sets the value of a type 0x02 transaction. Ignored for non-0x02
// types.
func (b *Builder) EthValue(value *big.Int) *Builder {
	if value != nil {
		b.ethValue = new(big.Int).Set(value)
	}
	return b
}

// EthData sets the call data of a type 0x02 transaction. Ignored for
// non-0x02 types.
func (b *Builder) EthData(data []byte) *Builder {
	b.ethData = append([]byte(nil), data...)
	return b
}

// AccessList sets the EIP-2930 access list of a type 0x02 transaction. The
// list is carried bit-exact for hash compatibility, but non-empty lists are
// rejected at runtime resolution.
func (b *Builder) AccessList(list AccessList) *Builder {
	b.accessList = append(AccessList(nil), list...)
	return b
}

// Build builds a tx object.
func (b *Builder) Build() *Transaction {
	if b.txType == TypeLegacy {
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
	}

	if b.txType == TypeEthDynamicFee {
		value := b.ethValue
		if value == nil {
			value = new(big.Int)
		}
		maxFee := b.maxFeePerGas
		if maxFee == nil {
			maxFee = new(big.Int)
		}
		maxPrio := b.maxPriorityFeePerGas
		if maxPrio == nil {
			maxPrio = new(big.Int)
		}
		chainID := b.chainID
		if chainID == nil {
			chainID = new(big.Int)
		}
		return &Transaction{
			body: &ethDynamicFeeTransaction{
				ChainID:              chainID,
				Nonce:                b.nonce,
				MaxPriorityFeePerGas: maxPrio,
				MaxFeePerGas:         maxFee,
				Gas:                  b.gas,
				To:                   b.ethTo,
				Value:                value,
				Data:                 b.ethData,
				AccessList:           b.accessList,
				V:                    new(big.Int),
				R:                    new(big.Int),
				S:                    new(big.Int),
			},
		}
	}

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
