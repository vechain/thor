// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"bytes"
	"errors"
	"github.com/vechain/thor/v2/thor"
	"math/big"
)

// Uint256 is a wrapper for storage and retrieval of an uint256. Similar to storing an uint256 in a smart contract.
// It can also be accessed directly in the relevant built-in contract if declared in the same `pos`
// If the provided uint exceeds 256 bits, it will be truncated to fit into thor.Bytes32
type Uint256 struct {
	context *Context
	pos     thor.Bytes32
}

func NewUint256(context *Context, slot thor.Bytes32) *Uint256 {
	return &Uint256{context: context, pos: slot}
}

func (u *Uint256) Get() (*big.Int, error) {
	storage, err := u.context.state.GetStorage(u.context.address, u.pos)
	if err != nil {
		return nil, err
	}
	u.context.UseGas(thor.SloadGas)
	return new(big.Int).SetBytes(storage.Bytes()), nil
}

var maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

func (u *Uint256) Set(value *big.Int) error {
	storage := thor.BytesToBytes32(value.Bytes())
	if value.Sign() == -1 {
		// provide extra context in error message by providing slot name
		key := string(bytes.TrimLeft(u.pos[:], string([]byte{0x00})))
		return errors.New(key + " uint256 cannot be negative")
	}
	if value.Cmp(maxUint256) > 0 {
		return errors.New("uint256 overflow: value exceeds 256 bits")
	}
	u.context.UseGas(thor.SstoreResetGas)
	u.context.state.SetStorage(u.context.address, u.pos, storage)
	return nil
}

func (u *Uint256) Add(value *big.Int) error {
	if value.Sign() == 0 {
		return nil
	}
	storage, err := u.Get()
	if err != nil {
		return err
	}
	storage.Add(storage, value)
	return u.Set(storage)
}

func (u *Uint256) Sub(value *big.Int) error {
	if value.Sign() == 0 {
		return nil
	}
	storage, err := u.Get()
	if err != nil {
		return err
	}
	storage.Sub(storage, value)
	return u.Set(storage)
}
