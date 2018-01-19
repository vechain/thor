package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/thor"
)

type energy struct {
	contract
}

// PackConsume pack input data of `Energy.consume` function.
func (e *energy) PackConsume(caller thor.Address, callee thor.Address, amount *big.Int) []byte {
	return e.mustPack("consume", caller, callee, amount)
}

// UnpackConsume unpack return data of `Energy.consume` function.
func (e *energy) UnpackConsume(output []byte) thor.Address {
	var addr common.Address
	e.mustUnpack(&addr, "consume", output)
	return thor.Address(addr)
}

// PackCharge pack input data of `Energy.charge` function.
func (e *energy) PackCharge(receiver thor.Address, amount *big.Int) []byte {
	return e.mustPack("charge", receiver, amount)
}

// PackInitialize pack input data of `Energy.Initialize` function.
func (e *energy) PackInitialize(params thor.Address) []byte {
	return e.mustPack("initialize", params)
}

// Energy binder of `Energy` contract.
var Energy = energy{mustLoad(
	thor.BytesToAddress([]byte("eng")),
	"compiled/Energy.abi",
	"compiled/Energy.bin-runtime")}
