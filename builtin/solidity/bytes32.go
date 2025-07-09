// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Bytes32 is a wrapper for storage and retrieval of [32]byte
type Bytes32 struct {
	contract thor.Address
	state    *state.State
	pos      thor.Bytes32
	prev    thor.Bytes32 // Optional previous value for comparison
	charger *gascharger.Charger
}

func NewBytes32(root *Root, pos thor.Bytes32) *Bytes32 {
	return &Bytes32{root.address, root.state, pos, thor.Bytes32{}, root.charger}
}

func (a *Bytes32) Get() (thor.Bytes32, error) {
	val, err := a.state.GetStorage(a.contract, a.pos)
	if err != nil {
		return thor.Bytes32{}, err
	}
	a.charger.Charge(thor.SloadGas)
	a.prev = val // Store the previous value for potential comparison
	return val, nil
}

func (a *Bytes32) Set(bytes *thor.Bytes32) {
	if bytes == nil {
		bytes = &thor.Bytes32{}
	}
	if a.prev.IsZero() {
		a.charger.Charge(thor.SstoreSetGas)
	} else {
		a.charger.Charge(thor.SstoreResetGas)
	}
	a.state.SetStorage(a.contract, a.pos, *bytes)
}
