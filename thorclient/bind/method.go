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

type MethodBuilder struct {
	contract *Contract
	method   string
	args     []any
	vet      *big.Int
}

// WithValue implements MethodBuilder.WithValue.
func (b *MethodBuilder) WithValue(vet *big.Int) *MethodBuilder {
	b.vet = vet
	return b
}

// Call implements MethodBuilder.Call.
func (b *MethodBuilder) Call() *CallBuilder {
	return &CallBuilder{
		op: b,
	}
}

// Send implements MethodBuilder.Send.
func (b *MethodBuilder) Send() *SendBuilder {
	return &SendBuilder{
		op: b,
	}
}

// Clause implements Clause build.
func (b *MethodBuilder) Clause() (*tx.Clause, error) {
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
