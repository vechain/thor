package processor

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

// Context for ExecuteMsg.
type Context struct {
	price    *big.Int
	sender   acc.Address
	header   *block.Header
	gasLimit uint64
	txHash   cry.Hash

	state   Stater
	getHash func(uint64) cry.Hash
}

// NewContext return Context.
func NewContext(price *big.Int, sender acc.Address, header *block.Header, gasLimit uint64, txHash cry.Hash, state Stater, getHash func(uint64) cry.Hash) *Context {
	return &Context{
		price:    price,
		sender:   sender,
		header:   header,
		gasLimit: gasLimit,
		txHash:   txHash,
		state:    state,
		getHash:  getHash,
	}
}

// ExecuteMsg can handle a transaction.message without prepare and check.
func ExecuteMsg(msg tx.Message, config vm.Config, context *Context) (*vm.Output, *big.Int) {
	ctx := vm.Context{
		Origin:      context.sender,
		Beneficiary: context.header.Beneficiary(),
		BlockNumber: new(big.Int).SetUint64(uint64(context.header.Number())),
		Time:        new(big.Int).SetUint64(uint64(context.header.Timestamp())),
		GasLimit:    context.header.GasLimit(),
		GasPrice:    context.price,
		TxHash:      context.txHash,
		GetHash:     context.getHash,
	}
	mvm := vm.NewVM(ctx, context.state, config) // message virtual machine
	var (
		output      *vm.Output
		leftOverGas uint64
		refundx     *big.Int
	)
	initialGas := new(big.Int).SetUint64(context.gasLimit)

	if msg.To() == nil {
		output, leftOverGas, refundx = mvm.Create(context.sender, msg.Data(), context.gasLimit, msg.Value())
	} else {
		output, leftOverGas, refundx = mvm.Call(context.sender, *(msg.To()), msg.Data(), context.gasLimit, msg.Value())
	}

	leftOverGas = refundGas(context.state, context.sender, leftOverGas, refundx, initialGas, context.price)
	gasUsed := new(big.Int).Sub(initialGas, new(big.Int).SetUint64(leftOverGas))

	return output, gasUsed
}

func refundGas(state Stater, sender acc.Address, leftOverGas uint64, refundGas *big.Int, initialGas *big.Int, price *big.Int) uint64 {
	// Return eth for remaining gas to the sender account,
	// exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(leftOverGas), price)
	state.SetBalance(sender, new(big.Int).Add(state.GetBalance(sender), remaining))

	// Apply refund counter, capped to half of the used gas.
	gasUsed := new(big.Int).Sub(initialGas, new(big.Int).SetUint64(leftOverGas))
	uhalf := remaining.Div(gasUsed, big.NewInt(2))
	refund := math.BigMin(uhalf, refundGas)
	leftOverGas += refund.Uint64()
	state.SetBalance(sender, new(big.Int).Add(state.GetBalance(sender), refund.Mul(refund, price)))

	return leftOverGas
}
