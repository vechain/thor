package processor

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/tx"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

type context struct {
	sender   acc.Address
	price    *big.Int
	gasLimit *big.Int
	gp       *core.GasPool // block gas limit
	txHash   cry.Hash
}

// Processor is a facade for vm.
// It can process the transactioner interface.
type Processor struct {
	state   Stater
	getHash func(uint64) cry.Hash

	context

	restGas    uint64
	refundGas  *big.Int
	initialGas *big.Int
}

// New is the Processor's Factory.
func New(st Stater, getHash func(uint64) cry.Hash) *Processor {
	return &Processor{
		state:      st,
		getHash:    getHash,
		refundGas:  new(big.Int),
		initialGas: new(big.Int),
	}
}

// Process transaction.
func (p *Processor) Process(header *block.Header, transaction *tx.Transaction, config vm.Config) ([]*VMOutput, *big.Int, error) {
	if err := p.initContext(transaction); err != nil {
		return nil, nil, err
	}

	// process message
	messages, _ := transaction.AsMessages()
	result := make([]*VMOutput, len(messages))
	for index, message := range messages {
		result[index] = p.processMessage(message, uint64(index), header, config)
		if err := result[index].ApplyState(p.state); err != nil {
			return nil, nil, err
		}
		if result[index].VMErr != nil {
			break
		}
	}

	// process refund
	p.refund()

	return result, p.gasUsed(), nil
}

func (p *Processor) initContext(transaction *tx.Transaction) error {
	// initialize context.
	messages, err := transaction.AsMessages()
	if err != nil {
		return err
	}

	p.price = transaction.GasPrice()
	sender, err := transaction.Signer()
	if err != nil {
		return err
	}
	p.sender = *sender
	p.gasLimit = transaction.GasLimit()
	p.gp = new(core.GasPool).AddGas(p.gasLimit)
	p.txHash = transaction.Hash()

	// verify context.
	return p.prepare(messages)
}

func (p *Processor) processMessage(msg tx.Message, msgIndex uint64, header *block.Header, config vm.Config) *VMOutput {
	ctx := vm.Context{
		Origin:      p.sender,
		Beneficiary: header.Beneficiary(),
		BlockNumber: new(big.Int).SetUint64(uint64(header.Number())),
		Time:        new(big.Int).SetUint64(uint64(header.Timestamp())),
		GasLimit:    header.GasLimit(),
		GasPrice:    p.price,
		TxHash:      p.txHash,
		GetHash:     p.getHash,
		ClauseIndex: msgIndex,
	}
	mvm := vm.NewVM(ctx, p.state, config) // message virtual machine
	var (
		output    *vm.Output
		refundGas *big.Int
	)

	if msg.To() == nil {
		output, p.restGas, refundGas = mvm.Create(p.sender, msg.Data(), p.restGas, msg.Value())
	} else {
		output, p.restGas, refundGas = mvm.Call(p.sender, *(msg.To()), msg.Data(), p.restGas, msg.Value())
	}
	p.refundGas = new(big.Int).Add(p.refundGas, refundGas)

	return (*VMOutput)(output)
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

	currentBalance := p.state.GetBalance(p.sender)

	if currentBalance.Cmp(mgval) < 0 {
		return errors.New("insufficient balance to pay for gas")
	}

	if err := p.gp.SubGas(amount); err != nil {
		return err
	}

	p.restGas += amount.Uint64()
	p.initialGas.Set(amount)
	p.state.SetBalance(p.sender, new(big.Int).Sub(currentBalance, mgval))

	return nil
}

func (p *Processor) refund() {
	// Return eth for remaining gas to the sender account,
	// exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(p.restGas), p.price)
	p.state.SetBalance(p.sender, new(big.Int).Add(p.state.GetBalance(p.sender), remaining))

	// Apply refund counter, capped to half of the used gas.
	uhalf := remaining.Div(p.gasUsed(), big.NewInt(2))
	refund := math.BigMin(uhalf, p.refundGas)
	p.state.SetBalance(p.sender, new(big.Int).Add(p.state.GetBalance(p.sender), refund.Mul(refund, p.price)))
	p.restGas += refund.Uint64()

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	p.gp.AddGas(new(big.Int).SetUint64(p.restGas))
}

func (p *Processor) gasUsed() *big.Int {
	return new(big.Int).Sub(p.initialGas, new(big.Int).SetUint64(p.restGas))
}
