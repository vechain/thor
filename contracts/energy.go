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

// PackUpdateBalance pack input data of `Energy.updateBalance` function.
func (e *energy) PackUpdateBalance(owner thor.Address) []byte {
	return e.mustPack("updateBalance", owner)
}

// PackUpdateBalance pack input data of `Energy.balanceOf` function.
func (e *energy) PackBalanceOf(owner thor.Address) []byte {
	return e.mustPack("balanceOf", owner)
}

// PackSetBalanceBirth pack input data of `Energy.setBalanceBirth` function.
func (e *energy) PackSetBalanceBirth(birth *big.Int) []byte {
	return e.mustPack("setBalanceBirth", birth)
}

// PackSetOwnerForContract pack input data of `Energy.setOwnerForContract` function.
func (e *energy) PackSetOwnerForContract(owner *thor.Address) []byte {
	return e.mustPack("setOwnerForContract", owner)
}

// Energy binder of `Energy` contract.
var Energy = energy{mustLoad(
	thor.BytesToAddress([]byte("eng")),
	"compiled/Energy.abi",
	"compiled/Energy.bin-runtime")}
