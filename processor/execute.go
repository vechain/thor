package processor

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/account"
)

// Context for ExecuteMsg.
type Context struct {
	price    *big.Int
	sender   acc.Address
	header   *block.Header
	gasLimit uint64
	txHash   cry.Hash

	state   account.StateReader
	storage account.StorageReader
	kv      account.KVReader
	getHash func(uint64) cry.Hash
}

// NewContext return Context.
func NewContext(price *big.Int, sender acc.Address, header *block.Header, gasLimit uint64, txHash cry.Hash, state account.StateReader, storage account.StorageReader, kv account.KVReader, getHash func(uint64) cry.Hash) *Context {
	return &Context{
		price:    price,
		sender:   sender,
		header:   header,
		gasLimit: gasLimit,
		txHash:   txHash,
		state:    state,
		storage:  storage,
		kv:       kv,
		getHash:  getHash,
	}
}

// ExecuteMsg can handle a transaction.message without prepare and check.
func ExecuteMsg(msg tx.Message, config vm.Config, context *Context) *vm.Output {
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
	am := account.NewManager(context.kv, context.state, context.storage)
	mvm := vm.NewVM(ctx, am, config) // message virtual machine
	var output *vm.Output
	initialGas := new(big.Int).SetUint64(context.gasLimit)

	if msg.To() == nil {
		output = mvm.Create(context.sender, msg.Data(), context.gasLimit, msg.Value())
	} else {
		output = mvm.Call(context.sender, *(msg.To()), msg.Data(), context.gasLimit, msg.Value())
	}

	if output.VMErr != nil {
		return output
	}

	output.LeftOverGas = refundGas(am, context.sender, output.LeftOverGas, initialGas, context.price)
	gasUsed := new(big.Int).Sub(initialGas, new(big.Int).SetUint64(output.LeftOverGas))
	am.AddBalance(context.header.Beneficiary(), new(big.Int).Mul(gasUsed, context.price))
	output.DirtiedAccounts = am.GetDirtyAccounts()

	return output
}

func refundGas(am *account.Manager, sender acc.Address, leftOverGas uint64, initialGas *big.Int, price *big.Int) uint64 {
	// Return eth for remaining gas to the sender account,
	// exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(leftOverGas), price)
	am.AddBalance(sender, remaining)

	// Apply refund counter, capped to half of the used gas.
	gasUsed := new(big.Int).Sub(initialGas, new(big.Int).SetUint64(leftOverGas))
	uhalf := remaining.Div(gasUsed, big.NewInt(2))
	refund := math.BigMin(uhalf, am.GetRefund())
	leftOverGas += refund.Uint64()

	am.AddBalance(sender, refund.Mul(refund, price))

	return leftOverGas
}
