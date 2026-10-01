// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package contracts holds dependency-free metadata (address and ABI) for
// builtin contracts, extracted so it can be imported without pulling in
// state/chain/muxdb.
package contracts

import (
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin/gen"
	"github.com/vechain/thor/v2/thor"
)

// Contract is the metadata of a builtin contract. Use the package-level
// variables: a zero Contract has no compiled assets and its methods panic.
type Contract struct {
	name    string
	Address thor.Address
	ABI     *abi.ABI
}

func mustLoad(name string) *Contract {
	return mustLoadAt(name, thor.BytesToAddress([]byte(name)))
}

// mustLoadAt loads a builtin contract whose deployed address is fixed
// (i.e. not derived from its name). Used for contracts that follow an
// externally specified address such as EIP-2935 HISTORY_STORAGE.
func mustLoadAt(name string, address thor.Address) *Contract {
	asset := "compiled/" + name + ".abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load ABI for '"+name+"'"))
	}

	return &Contract{
		name,
		address,
		abi,
	}
}

// RuntimeBytecodes load runtime byte codes.
func (c *Contract) RuntimeBytecodes() []byte {
	asset := "compiled/" + c.name + ".bin-runtime"
	data := gen.MustBIN(asset)
	return data
}

// RawABI load raw ABI data.
func (c *Contract) RawABI() []byte {
	asset := "compiled/" + c.name + ".abi"
	data := gen.MustABI(asset)
	return data
}

func (c *Contract) NativeABI() *abi.ABI {
	asset := "compiled/" + c.name + "Native.abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load native ABI for '"+c.name+"'"))
	}
	return abi
}

// Builtin contracts metadata.
var (
	Params      = mustLoad("Params")
	Authority   = mustLoad("Authority")
	Energy      = mustLoad("Energy")
	Executor    = mustLoad("Executor")
	Prototype   = mustLoad("Prototype")
	Extension   = mustLoad("Extension")
	ExtensionV2 = mustLoad("ExtensionV2")
	ExtensionV3 = mustLoad("ExtensionV3")
	Staker      = mustLoad("Staker")
	Measure     = mustLoad("Measure")

	// History is the EIP-2935 historical-block-hash facade. Its address is
	// fixed by the EIP (not derived from the contract name) so dApps that
	// already speak the EIP-2935 calling convention work unchanged on Thor.
	History = mustLoadAt("History", thor.MustParseAddress("0x0000F90827F1C53a10cb7A02335B175320002935"))
)
