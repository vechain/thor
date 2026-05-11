// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package statedb

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// stateDBV2 inherits V1 and overrides Get/SetNonce to read/persist the
// on-state account nonce for eth tx execution.
type stateDBV2 struct {
	*stateDB
}

// NewV2 creates a V2 statedb.
func NewV2(state *state.State) StateDB {
	return &stateDBV2{stateDB: newV1(state)}
}

// GetNonce reads the on-state nonce, shadowing V1's zero stub.
func (s *stateDBV2) GetNonce(addr common.Address) uint64 {
	n, err := s.state.GetNonce(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	return n
}

// SetNonce persists the nonce, shadowing V1's no-op.
func (s *stateDBV2) SetNonce(addr common.Address, nonce uint64) {
	if err := s.state.SetNonce(thor.Address(addr), nonce); err != nil {
		panic(err)
	}
}
