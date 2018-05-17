// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin/authority"
	"github.com/vechain/thor/builtin/energy"
	"github.com/vechain/thor/builtin/extension"
	"github.com/vechain/thor/builtin/gen"
	"github.com/vechain/thor/builtin/params"
	"github.com/vechain/thor/builtin/prototype"
	"github.com/vechain/thor/state"
)

// Builtin contracts binding.
var (
	Params    = &paramsContract{mustLoadContract("Params")}
	Authority = &authorityContract{mustLoadContract("Authority")}
	Energy    = &energyContract{mustLoadContract("Energy")}
	Executor  = &executorContract{mustLoadContract("Executor")}
	Prototype = &prototypeContract{
		mustLoadContract("Prototype"),
		mustLoadPrototypeInterfaceABI(),
	}
	Extension = &extensionContract{mustLoadContract("Extension")}
)

type (
	paramsContract    struct{ *contract }
	authorityContract struct{ *contract }
	energyContract    struct{ *contract }
	executorContract  struct{ *contract }
	prototypeContract struct {
		*contract
		InterfaceABI *abi.ABI
	}
	extensionContract struct{ *contract }
)

func (p *paramsContract) Native(state *state.State) *params.Params {
	return params.New(p.Address, state)
}

func (a *authorityContract) Native(state *state.State) *authority.Authority {
	return authority.New(a.Address, state)
}

func (e *energyContract) Native(state *state.State) *energy.Energy {
	return energy.New(e.Address, state)
}

func (p *prototypeContract) Native(state *state.State) *prototype.Prototype {
	return prototype.New(p.Address, state)
}

func (e *extensionContract) Native(state *state.State) *extension.Extension {
	return extension.New(e.Address, state)
}

func mustLoadPrototypeInterfaceABI() *abi.ABI {
	asset := "compiled/PrototypeInterface.abi"
	data := gen.MustAsset(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load native ABI for PrototypeInterface"))
	}
	return abi
}
