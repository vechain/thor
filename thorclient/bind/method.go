// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"fmt"

	"math/big"

	"github.com/vechain/thor/v2/tx"
)

// MethodBuilder is the interface that routes to specific operation types.
type MethodBuilder interface {
	// WithValue sets the VET value for the operation.
	WithValue(vet *big.Int) MethodBuilder

	// Call returns a CallBuilder for read operations.
	Call() CallBuilder

	// Send returns a SendBuilder for write operations.
	Send() SendBuilder

	// Clause returns a ClauseBuilder for building transaction clauses.
	Clause() (*tx.Clause, error)
}

// methodBuilder is the concrete implementation of MethodBuilder.
type methodBuilder struct {
	contract *contract
	method   string
	args     []any
	vet      *big.Int
}

// WithValue implements MethodBuilder.WithValue.
func (b *methodBuilder) WithValue(vet *big.Int) MethodBuilder {
	b.vet = vet
	return b
}

// Call implements MethodBuilder.Call.
func (b *methodBuilder) Call() CallBuilder {
	return &callBuilder{
		op: b,
	}
}

// Send implements MethodBuilder.Send.
func (b *methodBuilder) Send() SendBuilder {
	return &sendBuilder{
		op: b,
	}
}

// Clause implements Clause build.
func (b *methodBuilder) Clause() (*tx.Clause, error) {
	method, ok := b.contract.abi.Methods[b.method]
	if !ok {
		return nil, fmt.Errorf("method not found: %s", b.method)
	}

	data, err := method.Inputs.Pack(b.args...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack method (%s): %w", b.method, err)
	}

	data = append(method.Id()[:], data...)
	clause := tx.NewClause(b.contract.addr).WithData(data).WithValue(b.vet)

	return clause, nil
}
