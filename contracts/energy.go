package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Energy binder of `Energy` contract.
var Energy = &energy{
	thor.BytesToAddress([]byte("eng")),
	mustLoadABI("compiled/Energy.abi"),
}

type energy struct {
	Address thor.Address
	abi     *abi.ABI
}

func (e *energy) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Energy.bin-runtime")
}

// PackConsume pack input data of `Energy.consume` function.
func (e *energy) PackConsume(caller thor.Address, callee thor.Address, amount *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "consume", caller, callee, amount))
}

// UnpackConsume unpack return data of `Energy.consume` function.
func (e *energy) UnpackConsume(output []byte) thor.Address {
	var addr common.Address
	mustUnpack(e.abi, &addr, "consume", output)
	return thor.Address(addr)
}

// PackCharge pack input data of `Energy.charge` function.
func (e *energy) PackCharge(receiver thor.Address, amount *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "charge", receiver, amount))
}

// PackInitialize pack input data of `Energy.Initialize` function.
func (e *energy) PackInitialize(params thor.Address) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "initialize", params))
}

// PackUpdateBalance pack input data of `Energy.updateBalance` function.
func (e *energy) PackUpdateBalance(owner thor.Address) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "updateBalance", owner))
}

// PackUpdateBalance pack input data of `Energy.balanceOf` function.
func (e *energy) PackBalanceOf(owner thor.Address) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "balanceOf", owner))
}

// PackSetBalanceBirth pack input data of `Energy.setBalanceBirth` function.
func (e *energy) PackSetBalanceBirth(birth *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "setBalanceBirth", birth))
}

// PackTransfer pack input data of `Energy.transfer` function.
func (e *energy) PackTransfer(to thor.Address, amount *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "transfer", to, amount))
}

// PackTransferFrom pack input data of `Energy.transferFrom` function.
func (e *energy) PackTransferFrom(from thor.Address, to thor.Address, amount *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "transferFrom", from, to, amount))
}

// PackSetOwnerForContract pack input data of `Energy.setOwnerForContract` function.
func (e *energy) PackSetOwnerForContract(contractAddr thor.Address, owner thor.Address) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "setOwnerForContract", contractAddr, owner))
}

// PackOwnerApprove pack input data of `Energy.ownerApprove` function.
func (e *energy) PackOwnerApprove(contractAddr thor.Address, reciever thor.Address, amount *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "ownerApprove", contractAddr, reciever, amount))
}

// PackApprove pack input data of `Energy.approve` function.
func (e *energy) PackApprove(reciever thor.Address, amount *big.Int) *tx.Clause {
	return tx.NewClause(&e.Address).
		WithData(mustPack(e.abi, "approve", reciever, amount))
}
