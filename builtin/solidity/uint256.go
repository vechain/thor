// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"math/big"

	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Uint256 is a wrapper for storage and retrieval of an uint256. Similar to storing an uint256 in a smart contract.
// It can also be accessed directly in the relevant built-in contract if declared in the same `pos`
// If the provided uint exceeds 256 bits, it will be truncated to fit into thor.Bytes32
type Uint256 struct {
	addr  thor.Address
	pos   thor.Bytes32
	state *state.State
}

func NewUint256(addr thor.Address, state *state.State, slot thor.Bytes32) *Uint256 {
	return &Uint256{addr: addr, state: state, pos: slot}
}

func (u *Uint256) Get() (value *big.Int, err error) {
	storage, err := u.state.GetStorage(u.addr, u.pos)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(storage.Bytes()), nil
}

func (u *Uint256) Set(value *big.Int) {
	storage := thor.BytesToBytes32(value.Bytes())
	u.state.SetStorage(u.addr, u.pos, storage)
}

func (u *Uint256) Add(value *big.Int) error {
	storage, err := u.Get()
	if err != nil {
		return err
	}
	storage.Add(storage, value)
	u.Set(storage)
	return nil
}

func (u *Uint256) Sub(value *big.Int) error {
	storage, err := u.Get()
	if err != nil {
		return err
	}
	storage.Sub(storage, value)
	u.Set(storage)
	return nil
}
