// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
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

	// Clause returns a ClauseBuilder for building transaction clauses.
	Clause() ClauseBuilder
}

// operationBuilder is the concrete implementation of OperationBuilder.
type operationBuilder struct {
	contract *contract
	method   string
	args     []any
	vet      *big.Int
}

// newOperationBuilder creates a new operation builder.
func newOperationBuilder(contract *contract, method string, args ...any) *operationBuilder {
	return &operationBuilder{
		contract: contract,
		method:   method,
		args:     args,
		vet:      big.NewInt(0),
	}
}

// WithValue implements OperationBuilder.WithValue.
func (b *operationBuilder) WithValue(vet *big.Int) OperationBuilder {
	b.vet = vet
	return b
}

// Call implements OperationBuilder.Call.
func (b *operationBuilder) Call() CallBuilder {
	if _, ok := b.contract.abi.Methods[b.method]; !ok {
		// Could panic or return error - design decision
		panic("method not found: " + b.method)
	}
	return newCallBuilder(b)
}

// Send implements OperationBuilder.Send.
func (b *operationBuilder) Send() SendBuilder {
	return newSendBuilder(b)
}

// Clause implements OperationBuilder.Clause.
func (b *operationBuilder) Clause() ClauseBuilder {
	return newClauseBuilder(b)
}

// Filter implements OperationBuilder.Filter.
func (b *operationBuilder) Filter() FilterBuilder {
	return newFilterBuilder(b)
}
