// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/thor"
)

// Address is a wrapper for storage and retrieval of address
type Address struct {
	context *Context
	pos     thor.Bytes32
}

func NewAddress(context *Context, pos thor.Bytes32) *Address {
	return &Address{context: context, pos: pos}
}

func (a *Address) Get() (*thor.Address, error) {
	storage, err := a.context.state.GetStorage(a.context.address, a.pos)
	if err != nil {
		return nil, err
	}
	a.context.UseGas(thor.SloadGas)
	address := thor.BytesToAddress(storage.Bytes())
	return &address, nil
}

func (a *Address) Set(address *thor.Address, newValue bool) {
	if address == nil {
		address = &thor.Address{}
	}
	if newValue {
		a.context.UseGas(thor.SstoreSetGas)
	} else {
		a.context.UseGas(thor.SstoreResetGas)
	}
	storage := thor.BytesToBytes32(address.Bytes())
	a.context.state.SetStorage(a.context.address, a.pos, storage)
}
