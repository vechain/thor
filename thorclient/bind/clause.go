// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/tx"
)

// ClauseBuilder is the interface for building transaction clauses.
type ClauseBuilder interface {
	// WithVET sets the VET value for the clause.
	WithVET(vet *big.Int) ClauseBuilder

	// Build builds and returns the clause.
	Build() (*tx.Clause, error)
}

// clauseBuilder is the concrete implementation of ClauseBuilder.
type clauseBuilder struct {
	op *operationBuilder
}

// WithVET implements ClauseBuilder.WithVET.
func (b *clauseBuilder) WithVET(vet *big.Int) ClauseBuilder {
	b.op.vet = vet
	return b
}

// Build implements ClauseBuilder.Build.
func (b *clauseBuilder) Build() (*tx.Clause, error) {
	method, ok := b.op.contract.abi.Methods[b.op.method]
	if !ok {
		return nil, errors.New("method not found: " + b.op.method)
	}

	data, err := method.Inputs.Pack(b.op.args...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack method (%s): %w", b.op.method, err)
	}

	data = append(method.Id()[:], data...)
	clause := tx.NewClause(b.op.contract.addr).WithData(data).WithValue(b.op.vet)

	return clause, nil
}
