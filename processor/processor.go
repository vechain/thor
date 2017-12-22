package processor

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/tx"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/log"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/account"
	"github.com/vechain/thor/vm/evm"
)

// Processor is a facade for vm.
// It can handle the transactioner interface.
type Processor struct {
	state   Stater
	am      *account.Manager
	getHash func(uint64) cry.Hash

	sender   acc.Address
	price    *big.Int
	gasLimit *big.Int
	gp       *core.GasPool // block gas limit
	txHash   cry.Hash

	restGas    uint64
	initialGas *big.Int
}

// New is the Processor's Factory.
func New(st Stater, kv account.KVReader, getHash func(uint64) cry.Hash) *Processor {
	am := account.NewManager(kv, st)
	return &Processor{
		state:      st,
		am:         am,
		getHash:    getHash,
		initialGas: new(big.Int),
	}
}

// Handle hanle the messages.
func (p *Processor) Handle(header *block.Header, transaction *tx.Transaction, config vm.Config) ([]*vm.Output, error) {
	// initialize context.
	messages, err := transaction.AsMessages()
	if err != nil {
		return nil, err
	}

	p.price = transaction.GasPrice()
	sender, _ := transaction.Signer()
	p.sender = *sender
	p.gp = new(core.GasPool).AddGas(transaction.GasLimit())
	p.gasLimit = transaction.GasLimit()
	p.txHash = transaction.Hash()

	// verify context.
	if err := p.prepare(messages); err != nil {
		return nil, err
	}

	// execute.
	result := make([]*vm.Output, len(messages))

	for index, message := range messages {
		output := p.handleUnitMsg(message, header, config)
		result[index] = output

		if output.VMErr != nil {
			log.Debug("VM returned with error", "err", output.VMErr)
			return result, nil
		}

		p.restGas = output.LeftOverGas
		p.refundGas()
		output.LeftOverGas = p.restGas

		p.am.AddBalance(header.Beneficiary(), new(big.Int).Mul(p.gasUsed(), p.price))
		output.DirtiedAccounts = p.am.GetDirtiedAccounts()

		p.updateState(output.DirtiedAccounts)
	}

	return result, nil
}

func (p *Processor) updateState(accounts []*account.Account) {
	for _, account := range accounts {
		p.state.UpdateAccount(account.Address, account.Data)
		for key, value := range account.Storage {
			p.state.UpdateStorage(key, value)
		}
	}
}

func (p *Processor) handleUnitMsg(msg tx.Message, header *block.Header, config vm.Config) *vm.Output {
	ctx := vm.Context{
		Origin:      p.sender,
		Beneficiary: header.Beneficiary(),
		BlockNumber: new(big.Int).SetUint64(uint64(header.Number())),
		Time:        new(big.Int).SetUint64(uint64(header.Timestamp())),
		GasLimit:    header.GasLimit(),
		GasPrice:    p.price,
		TxHash:      p.txHash,
		GetHash:     p.getHash,
	}
	mvm := vm.NewVM(ctx, p.am, config) // message virtual machine
	var output *vm.Output

	if msg.To() == nil {
		output = mvm.Create(p.sender, msg.Data(), p.restGas, msg.Value())
	} else {
		output = mvm.Call(p.sender, *(msg.To()), msg.Data(), p.restGas, msg.Value())
	}
	return output
}

// partial function for core.IntrinsicGas.
func (p *Processor) intrinsicGas(msg tx.Message) *big.Int {
	return core.IntrinsicGas(msg.Data(), msg.To() == nil, true)
}

func (p *Processor) prepare(messages []tx.Message) error {
	if p.gasLimit.BitLen() > 64*len(messages) {
		return evm.ErrOutOfGas
	}

	allIntrinsicGas := new(big.Int)
	for _, message := range messages {
		intrinsicGas := p.intrinsicGas(message)
		allIntrinsicGas = new(big.Int).Add(allIntrinsicGas, intrinsicGas)
	}

	if allIntrinsicGas.BitLen() > 64*len(messages) {
		return evm.ErrOutOfGas
	}

	if p.gasLimit.Uint64() < allIntrinsicGas.Uint64() {
		return evm.ErrOutOfGas
	}

	if err := p.buyGas(p.gasLimit); err != nil {
		return err
	}

	p.restGas -= allIntrinsicGas.Uint64()

	return nil
}

func (p *Processor) buyGas(amount *big.Int) error {
	mgval := new(big.Int).Mul(amount, p.price)

	if p.am.GetBalance(p.sender).Cmp(mgval) < 0 {
		return errors.New("insufficient balance to pay for gas")
	}

	if err := p.gp.SubGas(amount); err != nil {
		return err
	}

	p.restGas += amount.Uint64()
	p.initialGas.Set(amount)
	p.am.SubBalance(p.sender, mgval)

	return nil
}

func (p *Processor) refundGas() {
	// Return eth for remaining gas to the sender account,
	// exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(p.restGas), p.price)
	p.am.AddBalance(p.sender, remaining)

	// Apply refund counter, capped to half of the used gas.
	uhalf := remaining.Div(p.gasUsed(), big.NewInt(2))
	refund := math.BigMin(uhalf, p.am.GetRefund())
	p.restGas += refund.Uint64()

	p.am.AddBalance(p.sender, refund.Mul(refund, p.price))

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	p.gp.AddGas(new(big.Int).SetUint64(p.restGas))
}

func (p *Processor) gasUsed() *big.Int {
	return new(big.Int).Sub(p.initialGas, new(big.Int).SetUint64(p.restGas))
}
