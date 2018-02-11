package contracts

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/thor"
)

// Executor binder of `Executor` contract.
var Executor = func() *executor {
	addr := thor.BytesToAddress([]byte("exe"))
	return &executor{
		addr,
		mustLoadABI("compiled/Executor.abi"),
	}
}()

type executor struct {
	Address thor.Address
	ABI     *abi.ABI
}

func (e *executor) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Executor.bin-runtime")
}
