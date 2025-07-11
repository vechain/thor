// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/vechain/thor/v2/thor"
)

// Bytes32 is a wrapper for storage and retrieval of [32]byte
type Bytes32 struct {
	context *Context
	pos     thor.Bytes32
}

func NewBytes32(context *Context, pos thor.Bytes32) *Bytes32 {
	return &Bytes32{context: context, pos: pos}
}

func (a *Bytes32) Get() (thor.Bytes32, error) {
	a.context.UseGas(thor.SloadGas)
	return a.context.State.GetStorage(a.context.Address, a.pos)
}

func (a *Bytes32) Set(bytes *thor.Bytes32, newValue bool) {
	if bytes == nil {
		bytes = &thor.Bytes32{}
	}
	if newValue {
		a.context.UseGas(thor.SstoreSetGas)
	} else {
		a.context.UseGas(thor.SstoreResetGas)
	}
	a.context.State.SetStorage(a.context.Address, a.pos, *bytes)
}
