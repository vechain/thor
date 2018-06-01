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
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
	"github.com/vechain/thor/xenv"
)

// Builtin contracts binding.
var (
	Params    = &paramsContract{mustLoadContract("Params")}
	Authority = &authorityContract{mustLoadContract("Authority")}
	Energy    = &energyContract{mustLoadContract("Energy")}
	Executor  = &executorContract{mustLoadContract("Executor")}
	Prototype = &prototypeContract{
		mustLoadContract("Prototype"),
		mustLoadThorLibABI(),
	}
	Extension = &extensionContract{mustLoadContract("Extension")}
	Measure   = mustLoadContract("Measure")
)

type (
	paramsContract    struct{ *contract }
	authorityContract struct{ *contract }
	energyContract    struct{ *contract }
	executorContract  struct{ *contract }
	prototypeContract struct {
		*contract
		ThorLibABI *abi.ABI
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

func mustLoadThorLibABI() *abi.ABI {
	asset := "compiled/thor.abi"
	data := gen.MustAsset(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load native ABI for ThorLib"))
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

// HandleNativeCall entry of native methods implementation.
func HandleNativeCall(
	seeker *chain.Seeker,
	state *state.State,
	blockCtx *xenv.BlockContext,
	txCtx *xenv.TransactionContext,
	evm *evm.EVM,
	contract *evm.Contract,
	readonly bool,
) func() ([]byte, error) {
	methodID, err := abi.ExtractMethodID(contract.Input)
	if err != nil {
		return nil
	}

	var method *nativeMethod
	if contract.Address() == contract.Caller() {
		// private methods require caller == to
		method = nativeMethods[methodKey{thor.Address(contract.Address()), methodID}]
	}

	if method == nil {
		return nil
	}

	return xenv.New(method.abi, seeker, state, blockCtx, txCtx, evm, contract).Call(method.run, readonly)
}
