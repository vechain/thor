// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin/authority"
	"github.com/vechain/thor/builtin/energy"
	"github.com/vechain/thor/builtin/gen"
	"github.com/vechain/thor/builtin/params"
	"github.com/vechain/thor/builtin/prototype"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

// Builtin contracts binding.
var (
	Params    = &paramsContract{mustLoadContract("Params")}
	Authority = &authorityContract{mustLoadContract("Authority")}
	Energy    = &energyContract{mustLoadContract("Energy")}
	Executor  = &executorContract{mustLoadContract("Executor")}
	Prototype = &prototypeContract{mustLoadContract("Prototype")}
	Extension = &extensionContract{
		mustLoadContract("Extension"),
		mustLoadContract("ExtensionV2"),
	}
	Measure = mustLoadContract("Measure")
)

type (
	paramsContract    struct{ *contract }
	authorityContract struct{ *contract }
	energyContract    struct{ *contract }
	executorContract  struct{ *contract }
	prototypeContract struct{ *contract }
	extensionContract struct {
		*contract
		V2 *contract
	}
)

func (p *paramsContract) Native(state *state.State) *params.Params {
	return params.New(p.Address, state)
}

func (a *authorityContract) Native(state *state.State) *authority.Authority {
	return authority.New(a.Address, state)
}

func (e *energyContract) Native(state *state.State, blockTime uint64) *energy.Energy {
	return energy.New(e.Address, state, blockTime)
}

func (p *prototypeContract) Native(state *state.State) *prototype.Prototype {
	return prototype.New(p.Address, state)
}

func (p *prototypeContract) Events() *abi.ABI {
	asset := "compiled/PrototypeEvent.abi"
	data := gen.MustAsset(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load ABI for "+asset))
	}
	return abi
}

type nativeMethod struct {
	abi *abi.Method
	run func(env *xenv.Environment) []interface{}
}

type methodKey struct {
	thor.Address
	abi.MethodID
}

var nativeMethods = make(map[methodKey]*nativeMethod)

// FindNativeCall find native calls.
func FindNativeCall(to thor.Address, input []byte) (*abi.Method, func(*xenv.Environment) []interface{}, bool) {
	methodID, err := abi.ExtractMethodID(input)
	if err != nil {
		return nil, nil, false
	}

	method := nativeMethods[methodKey{to, methodID}]
	if method == nil {
		return nil, nil, false
	}
	return method.abi, method.run, true
}
