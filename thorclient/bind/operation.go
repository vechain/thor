// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"fmt"

	"github.com/vechain/thor/v2/tx"
	"math/big"
)

// OperationBuilder is the interface that routes to specific operation types.
type OperationBuilder interface {
	// WithValue sets the VET value for the operation.
	WithValue(vet *big.Int) OperationBuilder

	// Call returns a CallBuilder for read operations.
	Call() CallBuilder

	// Send returns a SendBuilder for write operations.
	Send() SendBuilder

	// Clause builds a clause for the operation.
	Clause() (*tx.Clause, error)
}

// operationBuilder is the concrete implementation of OperationBuilder.
type operationBuilder struct {
	contract *contract
	method   string
	args     []any
	vet      *big.Int
}

// WithValue implements OperationBuilder.WithValue.
func (b *operationBuilder) WithValue(vet *big.Int) OperationBuilder {
	b.vet = vet
	return b
}

// Call implements OperationBuilder.Call.
func (b *operationBuilder) Call() CallBuilder {
	return &callBuilder{
		op: b,
	}
}

// Send implements OperationBuilder.Send.
func (b *operationBuilder) Send() SendBuilder {
	return &sendBuilder{
		op: b,
	}
}

// Clause implements Clause build.
func (b *operationBuilder) Clause() (*tx.Clause, error) {
	method, ok := b.contract.abi.Methods[b.method]
	if !ok {
		return nil, fmt.Errorf("method not found: " + b.method)
	}

	data, err := method.Inputs.Pack(b.args...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack method (%s): %w", b.method, err)
	}

	data = append(method.Id()[:], data...)
	clause := tx.NewClause(b.contract.addr).WithData(data).WithValue(b.vet)

	return clause, nil
}
