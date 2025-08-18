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

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

type CallBuilder struct {
	op     *MethodBuilder
	rev    string
	caller *thor.Address
}

// AtRevision implements CallBuilder.AtRevision.
func (b *CallBuilder) AtRevision(rev string) *CallBuilder {
	b.rev = rev
	return b
}

// Caller implements CallBuilder.AtRevision.
func (b *CallBuilder) Caller(caller *thor.Address) *CallBuilder {
	b.caller = caller
	return b
}

// ExecuteInto implements CallBuilder.Into.
func (b *CallBuilder) ExecuteInto(result any) error {
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
func (b *CallBuilder) Execute() (*api.CallResult, error) {
	// Build the clause
	clause, err := b.op.Clause()
	if err != nil {
		return nil, err
	}

	body := &api.BatchCallData{
		Caller: b.caller,
		Clauses: api.Clauses{
			{
				To:    b.op.contract.addr,
				Data:  hexutil.Encode(clause.Data()),
				Value: (*math.HexOrDecimal256)(clause.Value()),
			},
		},
	}

	var res []*api.CallResult
	res, err = b.op.contract.client.InspectClauses(body, thorclient.Revision(b.rev))
	if err != nil {
		return nil, fmt.Errorf("failed to inspect clauses (%s): %w", b.op.String(), err)
	}

	if len(res) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(res))
	}

	result := res[0]
	if result.Reverted {
		message := fmt.Sprintf("contract call reverted (%s)", b.op.String())
		if result.Data != "" {
			decoded, err := hexutil.Decode(result.Data)
			if err != nil {
				return result, fmt.Errorf("failed to decode revert data: %w", err)
			}
			revertReason, err := UnpackRevert(decoded)
			if err == nil {
				message = fmt.Sprintf("%s: %s", message, revertReason)
			}
		}
		return result, errors.New(message)
	}

	if result.VMError != "" {
		return nil, fmt.Errorf("VM error: %s", result.VMError)
	}

	return result, nil
}
