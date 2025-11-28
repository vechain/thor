// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"fmt"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin/gen"
	"github.com/vechain/thor/v2/thor"
)

type contract struct {
	name    string
	Address thor.Address
	ABI     *abi.ABI
}

func mustLoadContract(name string) *contract {
	asset := "compiled/" + name + ".abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(fmt.Errorf("load ABI for '%v': %w", name, err))
	}

	return &contract{
		name,
		thor.BytesToAddress([]byte(name)),
		abi,
	}
}

// RuntimeBytecodes load runtime byte codes.
func (c *contract) RuntimeBytecodes() []byte {
	asset := "compiled/" + c.name + ".bin-runtime"
	data := gen.MustBIN(asset)
	return data
}

// RawABI load raw ABI data.
func (c *contract) RawABI() []byte {
	asset := "compiled/" + c.name + ".abi"
	data := gen.MustABI(asset)
	return data
}

func (c *contract) NativeABI() *abi.ABI {
	asset := "compiled/" + c.name + "Native.abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(fmt.Errorf("load native ABI for '%v': %w", c.name, err))
	}
	return abi
}
