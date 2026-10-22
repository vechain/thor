// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

// simulateClause simulates the execution of a single clause on top of the best block
// and returns the simulation result.
func simulateClause(client *thorclient.Client, caller thor.Address, clause *tx.Clause) (*api.CallResult, error) {
	var to *thor.Address
	if clause.To() != nil {
		addr := *clause.To()
		to = &addr
	}
	sim, err := client.InspectClauses(&api.BatchCallData{
		Caller: &caller,
		Clauses: api.Clauses{{
			To:    to,
			Data:  hexutil.Encode(clause.Data()),
			Value: (*math.HexOrDecimal256)(clause.Value()),
		}},
	}, thorclient.Revision("best"))
	if err != nil {
		return nil, err
	}
	if len(sim) != 1 {
		return nil, fmt.Errorf("expected 1 simulation result, got %d", len(sim))
	}
	return sim[0], nil
}

// estimateGas returns the gas limit for clause: intrinsic gas, plus the simulated
// execution gas, plus a margin when the clause runs code. The simulation runs with
// a large gas budget, but under an exact limit each nested CALL/CREATE keeps back
// 1/64 of the remaining gas (EIP-150) and can run out. The margin is 20% of the
// execution gas (about 11 nested levels), and at least 15k.
func estimateGas(client *thorclient.Client, caller thor.Address, clause *tx.Clause) (uint64, *api.CallResult, error) {
	gas, err := tx.IntrinsicGas(clause)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to calculate intrinsic gas: %w", err)
	}
	sim, err := simulateClause(client, caller, clause)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to simulate clause: %w", err)
	}
	gas += sim.GasUsed
	if clause.To() == nil || len(clause.Data()) > 0 {
		gas += max(sim.GasUsed/5, 15_000)
	}
	return gas, sim, nil
}
