package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Energy binder of `Energy` contract.
var Energy = func() energy {
	addr := thor.BytesToAddress([]byte("eng"))
	return energy{
		addr,
		mustLoad("compiled/Energy.abi", "compiled/Energy.bin-runtime"),
		tx.NewClause(&addr),
	}
}()

type energy struct {
	Address thor.Address
	contract
	clause *tx.Clause
}

// PackConsume pack input data of `Energy.consume` function.
func (e *energy) PackConsume(caller thor.Address, callee thor.Address, amount *big.Int) *tx.Clause {
	return e.clause.WithData(e.mustPack("consume", caller, callee, amount))
}

// UnpackConsume unpack return data of `Energy.consume` function.
func (e *energy) UnpackConsume(output []byte) thor.Address {
	var addr common.Address
	e.mustUnpack(&addr, "consume", output)
	return thor.Address(addr)
}

// PackCharge pack input data of `Energy.charge` function.
func (e *energy) PackCharge(receiver thor.Address, amount *big.Int) *tx.Clause {
	return e.clause.WithData(e.mustPack("charge", receiver, amount))
}

// PackInitialize pack input data of `Energy.Initialize` function.
func (e *energy) PackInitialize(params thor.Address) *tx.Clause {
	return e.clause.WithData(e.mustPack("initialize", params))
}

// PackUpdateBalance pack input data of `Energy.updateBalance` function.
func (e *energy) PackUpdateBalance(owner thor.Address) *tx.Clause {
	return e.clause.WithData(e.mustPack("updateBalance", owner))
}

// PackUpdateBalance pack input data of `Energy.balanceOf` function.
func (e *energy) PackBalanceOf(owner thor.Address) *tx.Clause {
	return e.clause.WithData(e.mustPack("balanceOf", owner))
}

// PackSetBalanceBirth pack input data of `Energy.setBalanceBirth` function.
func (e *energy) PackSetBalanceBirth(birth *big.Int) *tx.Clause {
	return e.clause.WithData(e.mustPack("setBalanceBirth", birth))
}

// PackSetOwnerForContract pack input data of `Energy.setOwnerForContract` function.
func (e *energy) PackSetOwnerForContract(owner thor.Address) *tx.Clause {
	return e.clause.WithData(e.mustPack("setOwnerForContract", owner))
}

// PackTransfer pack input data of `Energy.transfer` function.
func (e *energy) PackTransfer(to thor.Address, amount *big.Int) *tx.Clause {
	return e.clause.WithData(e.mustPack("transfer", to, amount))
}
