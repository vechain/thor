// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
	"github.com/vechain/thor/xenv"
)

type nativeMethod struct {
	abi *abi.Method
	run func(env *xenv.Environment) []interface{}
}

type methodKey struct {
	thor.Address
	abi.MethodID
}

const (
	blake2b256WordGas uint64 = 3
	blake2b256Gas     uint64 = 15
)

var privateMethods = make(map[methodKey]*nativeMethod)

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
		method = privateMethods[methodKey{thor.Address(contract.Address()), methodID}]
	}

	if method == nil {
		return nil
	}

	return xenv.New(method.abi, seeker, state, blockCtx, txCtx, evm, contract).Call(method.run, readonly)
}
