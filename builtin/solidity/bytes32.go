// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Bytes32 is a wrapper for storage and retrieval of [32]byte
type Bytes32 struct {
	contract thor.Address
	state    *state.State
	pos      thor.Bytes32
}

func NewBytes32(contract thor.Address, state *state.State, pos thor.Bytes32) *Bytes32 {
	return &Bytes32{contract: contract, state: state, pos: pos}
}

func (a *Bytes32) Get() (thor.Bytes32, error) {
	return a.state.GetStorage(a.contract, a.pos)
}

func (a *Bytes32) Set(bytes *thor.Bytes32) {
	if bytes == nil {
		bytes = &thor.Bytes32{}
	}
	a.state.SetStorage(a.contract, a.pos, *bytes)
}
