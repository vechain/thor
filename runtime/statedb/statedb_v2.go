// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package statedb

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// StateDBV2 is the post-INTERSTELLAR variant used exclusively for 0x02
// (TypeEthDynamicFee) tx execution. It embeds V1 and overrides only SetNonce
// so that the VM's sender-nonce bumps and EIP-158 contract-nonce initialization
// persist into account state (feeding sequential nonce semantics + eth-style
// CREATE address derivation — spec 3 §3.2).
//
// GetNonce is intentionally inherited from V1 since V1 already reads real
// state after spec 3. Every other method also comes from V1, which keeps the
// V1 / V2 behavior divergence to a single, auditable line.
type StateDBV2 struct {
	*StateDB
}

// NewV2 constructs a V2 StateDB wrapping the given state.
func NewV2(s *state.State) *StateDBV2 {
	return &StateDBV2{StateDB: New(s)}
}

// SetNonce writes the sequential nonce into account state. Panics on state
// errors to match the V1 convention (StateDB methods do not return errors).
func (s *StateDBV2) SetNonce(addr common.Address, nonce uint64) {
	if err := s.state.SetNonce(thor.Address(addr), nonce); err != nil {
		panic(err)
	}
}
