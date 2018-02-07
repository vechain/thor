package contracts

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/vechain/thor/contracts/rabi"
	"github.com/vechain/thor/contracts/sslot"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm/evm"
)

// Authority binder of `Authority` contract.
var Authority = func() *authority {
	addr := thor.BytesToAddress([]byte("poa"))
	abi := mustLoadABI("compiled/Authority.abi")
	return &authority{
		addr,
		abi,
		rabi.New(abi),
		sslot.New(addr, 100),
		sslot.New(addr, 101),
	}
}()

type authority struct {
	Address          thor.Address
	abi              *abi.ABI
	rabi             *rabi.ReversedABI
	proposers        *sslot.StorageSlot
	proposerIndexMap *sslot.StorageSlot
}

func (a *authority) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Authority.bin-runtime")
}

// PackInitialize pack input data of `Authority._initialize` function.
func (a *authority) PackInitialize() *tx.Clause {
	return tx.NewClause(&a.Address).
		WithData(mustPack(a.abi, "_initialize", Voting.Address))
}

// PackAuthorize pack input data of `Authority.authorize` function.
func (a *authority) PackAuthorize(addr thor.Address, identity string) *tx.Clause {
	return tx.NewClause(&a.Address).
		WithData(mustPack(a.abi, "authorize", addr, identity))
}

func (a *authority) nativeIndexOfProposer(state *state.State, addr thor.Address) (index uint32) {
	a.proposerIndexMap.Get(state, a.proposerIndexMap.MapKey(addr), (*stgUInt32)(&index))
	return
}

func (a *authority) NativeAddProposer(state *state.State, addr thor.Address) bool {
	if a.nativeIndexOfProposer(state, addr) > 0 {
		// aready exists
		return false
	}
	var arrayLen uint32
	a.proposers.Get(state, a.proposers.DataKey(), (*stgUInt32)(&arrayLen))

	// increase array len
	a.proposers.Set(state, a.proposers.DataKey(), stgUInt32(arrayLen+1))

	// append
	a.proposers.Set(state, a.proposers.IndexKey(arrayLen), stgProposer{Address: addr})

	a.proposerIndexMap.Set(state, a.proposerIndexMap.MapKey(addr), stgUInt32(arrayLen+1))
	return true
}

func (a *authority) NativeRemoveProposer(state *state.State, addr thor.Address) bool {
	index := a.nativeIndexOfProposer(state, addr)
	if index == 0 {
		// not found
		return false
	}
	a.proposerIndexMap.Set(state, a.proposerIndexMap.MapKey(addr), stgUInt32(0))

	var arrayLen uint32
	a.proposers.Get(state, a.proposers.DataKey(), (*stgUInt32)(&arrayLen))
	if arrayLen == index {
		// is the last elem
		a.proposers.Set(state, a.proposers.IndexKey(index-1), stgProposer{})
	} else {
		var last stgProposer
		// move last elem to gap of removed one
		a.proposers.Get(state, a.proposers.IndexKey(arrayLen-1), &last)
		a.proposers.Set(state, a.proposers.IndexKey(arrayLen-1), stgProposer{})
		a.proposers.Set(state, a.proposers.IndexKey(index-1), last)
	}

	a.proposers.Set(state, a.proposers.DataKey(), stgUInt32(arrayLen-1))
	return true

}

func (a *authority) NativeGetProposer(state *state.State, addr thor.Address) (bool, uint32) {
	indexMapKey := a.proposerIndexMap.MapKey(addr)
	var arrayIndex uint32
	a.proposerIndexMap.Get(state, indexMapKey, (*stgUInt32)(&arrayIndex))
	if arrayIndex == 0 {
		// not found
		return false, 0
	}
	var p poa.Proposer
	a.proposers.Get(state, a.proposers.IndexKey(arrayIndex-1), (*stgProposer)(&p))
	return true, p.Status
}

func (a *authority) NativeUpdateProposer(state *state.State, addr thor.Address, status uint32) bool {
	indexMapKey := a.proposerIndexMap.MapKey(addr)
	var arrayIndex uint32
	a.proposerIndexMap.Get(state, indexMapKey, (*stgUInt32)(&arrayIndex))
	if arrayIndex == 0 {
		// not found
		return false
	}
	a.proposers.Set(
		state,
		a.proposers.IndexKey(arrayIndex-1),
		stgProposer{addr, status})
	return true
}

func (a *authority) NativeGetProposers(state *state.State) []poa.Proposer {
	var plen uint32
	a.proposers.Get(state, a.proposers.DataKey(), (*stgUInt32)(&plen))
	proposers := make([]poa.Proposer, 0, plen)

	for i := uint32(0); i < plen; i++ {
		k := a.proposers.IndexKey(i)
		var p poa.Proposer
		a.proposers.Get(state, k, (*stgProposer)(&p))
		proposers = append(proposers, p)
	}
	return proposers
}

func (a *authority) HandleNative(state *state.State, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
	name, err := a.rabi.NameOf(input)
	if err != nil {
		return nil
	}
	switch name {
	case "nativeAddProposer":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != a.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreSetGas) {
				return nil, evm.ErrOutOfGas
			}
			var addr common.Address
			if err := a.rabi.UnpackInput(&addr, name, input); err != nil {
				return nil, err
			}
			return a.rabi.PackOutput(name, a.NativeAddProposer(state, thor.Address(addr)))
		}
	case "nativeRemoveProposer":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != a.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreClearGas) {
				return nil, evm.ErrOutOfGas
			}
			var addr common.Address
			if err := a.rabi.UnpackInput(&addr, name, input); err != nil {
				return nil, err
			}
			return a.rabi.PackOutput(name, a.NativeRemoveProposer(state, thor.Address(addr)))
		}
	case "nativeGetProposer":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			var addr common.Address
			if err := a.rabi.UnpackInput(&addr, name, input); err != nil {
				return nil, err
			}
			found, status := a.NativeGetProposer(state, thor.Address(addr))
			return a.rabi.PackOutput(name, found, status)
		}
	}
	return nil
}
