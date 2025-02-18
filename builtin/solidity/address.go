// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Address is a wrapper for storage and retrieval of an address. Similar to storing an address in a smart contract.
// It can also be accessed directly in the relevant built-in contract if declared in the same `pos`
type Address struct {
	contract thor.Address
	state    *state.State
	pos      thor.Bytes32
}

func NewAddress(contract thor.Address, state *state.State, pos thor.Bytes32) *Address {
	return &Address{contract: contract, state: state, pos: pos}
}

func (a *Address) Get() (thor.Address, error) {
	storage, err := a.state.GetStorage(a.contract, a.pos)
	if err != nil {
		return thor.Address{}, err
	}
	return thor.BytesToAddress(storage.Bytes()), nil
}

func (a *Address) Set(addr *thor.Address) {
	var storage thor.Bytes32
	if addr != nil {
		storage = thor.BytesToBytes32(addr.Bytes())
	}
	a.state.SetStorage(a.contract, a.pos, storage)
}
