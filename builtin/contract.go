// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/pkg/errors"

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
	return mustLoadContractAt(name, thor.BytesToAddress([]byte(name)))
}

// mustLoadContractAt loads a builtin contract whose deployed address is fixed
// (i.e. not derived from its name). Used for contracts that follow an
// externally specified address such as EIP-2935 HISTORY_STORAGE.
func mustLoadContractAt(name string, address thor.Address) *contract {
	asset := "compiled/" + name + ".abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load ABI for '"+name+"'"))
	}

	return &contract{
		name,
		address,
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
		panic(errors.Wrap(err, "load native ABI for '"+c.name+"'"))
	}
	return abi
}
