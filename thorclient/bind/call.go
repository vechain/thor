// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// CallBuilder is the interface for read operations.
type CallBuilder interface {
	// AtRevision sets the revision for the call.
	AtRevision(rev string) CallBuilder

	// Caller sets the caller for the simulation.
	Caller(caller *thor.Address) CallBuilder

	// ExecuteInto unpacks the result into the provided interface.
	ExecuteInto(result any) error

	// Execute performs the call and returns the raw result.
	Execute() (*accounts.CallResult, error)
}

// callBuilder is the concrete implementation of CallBuilder.
type callBuilder struct {
	op     *methodBuilder
	rev    string
	caller *thor.Address
}

// AtRevision implements CallBuilder.AtRevision.
func (b *callBuilder) AtRevision(rev string) CallBuilder {
	b.rev = rev
	return b
}

// Caller implements CallBuilder.AtRevision.
func (b *callBuilder) Caller(caller *thor.Address) CallBuilder {
	b.caller = caller
	return b
}

// ExecuteInto implements CallBuilder.Into.
func (b *callBuilder) ExecuteInto(result any) error {
	method, ok := b.op.contract.abi.Methods[b.op.method]
	if !ok {
		return errors.New("method not found: " + b.op.method)
	}

	res, err := b.Execute()
	if err != nil {
		return err
	}

	bytes, err := hexutil.Decode(res.Data)
	if err != nil {
		return err
	}

	return method.Outputs.Unpack(result, bytes)
}

// Execute implements CallBuilder.Execute.
func (b *callBuilder) Execute() (*accounts.CallResult, error) {
	// Build the clause
	clause, err := b.op.Clause()
	if err != nil {
		return nil, err
	}

	body := &accounts.BatchCallData{
		Caller: b.caller,
		Clauses: []accounts.Clause{
			{
				To:    b.op.contract.addr,
				Data:  hexutil.Encode(clause.Data()),
				Value: (*math.HexOrDecimal256)(clause.Value()),
			},
		},
	}

	var res []*accounts.CallResult
	res, err = b.op.contract.client.InspectClauses(body, thorclient.Revision(b.rev))
	if err != nil {
		return nil, err
	}

	if len(res) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(res))
	}

	if res[0].Reverted {
		message := "contract call reverted"
		if res[0].Data != "" {
			decoded, err := hexutil.Decode(res[0].Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode revert data: %w", err)
			}
			revertReason, err := UnpackRevert(decoded)
			if err == nil {
				message = fmt.Sprintf("contract call reverted: %s", revertReason)
			}
		}
		return nil, errors.New(message)
	}

	if res[0].VMError != "" {
		return nil, fmt.Errorf("VM error: %s", res[0].VMError)
	}

	return res[0], nil
}
