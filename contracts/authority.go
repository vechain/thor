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
		sslot.NewArray(addr, 100),
		sslot.NewMap(addr, 101),
		sslot.NewMap(addr, 102),
	}
}()

type authority struct {
	Address     thor.Address
	ABI         *abi.ABI
	rabi        *rabi.ReversedABI
	array       *sslot.Array
	indexMap    *sslot.Map
	identityMap *sslot.Map
}

// RuntimeBytecodes load runtime byte codes.
func (a *authority) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Authority.bin-runtime")
}

func (a *authority) indexOfProposer(state *state.State, addr thor.Address) (index uint64) {
	a.indexMap.ForKey(addr).Load(state, &index)
	return
}

func (a *authority) AddProposer(state *state.State, addr thor.Address, identity thor.Hash) bool {
	if a.indexOfProposer(state, addr) > 0 {
		// aready exists
		return false
	}
	length := a.array.Append(state, &stgProposer{Address: addr})
	a.indexMap.ForKey(addr).Save(state, length)

	a.identityMap.ForKey(addr).Save(state, identity)
	return true
}

func (a *authority) RemoveProposer(state *state.State, addr thor.Address) bool {
	index := a.indexOfProposer(state, addr)
	if index == 0 {
		// not found
		return false
	}
	a.indexMap.ForKey(addr).Save(state, nil)
	length := a.array.Len(state)
	if length != index {
		var last stgProposer
		// move last elem to gap of removed one
		a.array.ForIndex(length-1).Load(state, &last)
		a.array.ForIndex(index-1).Save(state, &last)
	}
	a.array.SetLen(state, length-1)
	a.identityMap.ForKey(addr).Save(state, nil)
	return true

}

func (a *authority) GetProposer(state *state.State, addr thor.Address) (ok bool, identity thor.Hash, status uint32) {
	index := a.indexOfProposer(state, addr)
	if index == 0 {
		// not found
		return false, thor.Hash{}, 0
	}

	var p stgProposer
	a.array.ForIndex(index-1).Load(state, &p)

	a.identityMap.ForKey(addr).Load(state, &identity)
	return false, identity, p.Status
}

// UpdateProposer update proposer status.
func (a *authority) UpdateProposer(state *state.State, addr thor.Address, status uint32) bool {
	index := a.indexOfProposer(state, addr)
	if index == 0 {
		// not found
		return false
	}
	a.array.ForIndex(index-1).Save(state, &stgProposer{addr, status})
	return true
}

// GetProposers get proposers list.
func (a *authority) GetProposers(state *state.State) []poa.Proposer {
	length := a.array.Len(state)
	proposers := make([]poa.Proposer, 0, length)
	for i := uint64(0); i < length; i++ {
		var p stgProposer
		a.array.ForIndex(i).Load(state, &p)
		proposers = append(proposers, poa.Proposer(p))
	}
	return proposers
}

// HandleNative helper method to hook VM contract calls.
func (a *authority) HandleNative(state *state.State, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
	name, err := a.rabi.NameOf(input)
	if err != nil {
		return nil
	}
	switch name {
	case "nativeGetExecutor":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			return a.rabi.PackOutput(name, Executor.Address)
		}
	case "nativeAddProposer":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			// permission check
			if caller != a.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreSetGas) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				Addr     common.Address
				Identity common.Hash
			}
			if err := a.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			return a.rabi.PackOutput(name, a.AddProposer(state, thor.Address(args.Addr), thor.Hash(args.Identity)))
		}
	case "nativeRemoveProposer":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			// permission check
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
			return a.rabi.PackOutput(name, a.RemoveProposer(state, thor.Address(addr)))
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
			found, identity, status := a.GetProposer(state, thor.Address(addr))
			return a.rabi.PackOutput(name, found, identity, status)
		}
	}
	return nil
}
