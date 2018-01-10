package runtime

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

// Runtime is to support transaction execution.
type Runtime struct {
	vmConfig vm.Config
	getHash  func(uint64) thor.Hash
	state    State

	// block env
	blockBeneficiary thor.Address
	blockNumber      uint32
	blockTime        uint64
	blockGasLimit    uint64

	// tx env
	txOrigin   thor.Address
	txGasPrice bn.Int
	txHash     thor.Hash
}

// State to decouple state.State.
type State interface {
	vm.State
	RevertTo(revision int)
}

// New create a Runtime object.
func New(
	state State,
	header *block.Header,
	getHash func(uint64) thor.Hash,
) *Runtime {
	return &Runtime{
		getHash:       getHash,
		state:         state,
		txOrigin:      header.Beneficiary(),
		blockNumber:   header.Number(),
		blockTime:     header.Timestamp(),
		blockGasLimit: header.GasLimit(),
	}
}

// SetTransactionEnvironment set transaction related vars.
// Returns this runtime.
func (rt *Runtime) SetTransactionEnvironment(
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txHash thor.Hash) *Runtime {

	rt.txOrigin = txOrigin
	rt.txGasPrice.SetBig(txGasPrice)
	rt.txHash = txHash
	return rt
}

// SetVMConfig config VM.
// Returns this runtime.
func (rt *Runtime) SetVMConfig(config vm.Config) *Runtime {
	rt.vmConfig = config
	return rt
}

// Execute executes single clause bases on tx env set by SetTransactionEnvironment.
func (rt *Runtime) Execute(clause *Tx.Clause, index int, gas uint64) *vm.Output {

	ctx := vm.Context{
		Beneficiary: rt.blockBeneficiary,
		BlockNumber: new(big.Int).SetUint64(uint64(rt.blockNumber)),
		Time:        new(big.Int).SetUint64(rt.blockTime),
		GasLimit:    new(big.Int).SetUint64(rt.blockGasLimit),

		Origin:   rt.txOrigin,
		GasPrice: rt.txGasPrice.ToBig(),
		TxHash:   rt.txHash,

		GetHash:     rt.getHash,
		ClauseIndex: uint64(index),
	}

	if clause.To == nil {
		return vm.New(ctx, rt.state, rt.vmConfig).
			Create(rt.txOrigin, clause.Data, gas, clause.Value.ToBig())
	}
	return vm.New(ctx, rt.state, rt.vmConfig).
		Call(rt.txOrigin, *clause.To, clause.Data, gas, clause.Value.ToBig())
}

func (rt *Runtime) consumeEnergy(target *thor.Address, amount *big.Int) (thor.Address, error) {
	return thor.Address{}, nil
}

func (rt *Runtime) chargeEnergy(addr thor.Address, amount *big.Int) error {
	return nil
}

// ExecuteTransaction executes a transaction.
// If an error returned, state will not be affected.
// It will invoke SetTransactionEnvironment to reset tx env.
// Note that the elements of returned []*vm.Output may be nil if failed
// to execute corresponded clauses.
func (rt *Runtime) ExecuteTransaction(tx *Tx.Transaction) (*Tx.Receipt, []*vm.Output, error) {
	// precheck
	origin, err := tx.Signer()
	if err != nil {
		return nil, nil, err
	}
	intrinsicGas, err := tx.IntrinsicGas()
	if err != nil {
		return nil, nil, err
	}
	gas := tx.Gas()
	if intrinsicGas > gas {
		return nil, nil, errors.New("intrinsic gas exceeds provided gas")
	}

	gasPrice := tx.GasPrice().ToBig()

	// set tx env
	rt.SetTransactionEnvironment(origin, gasPrice, tx.Hash())

	clauses := tx.Clauses()
	commonTarget := commonTo(clauses)

	energyPrepayed := new(big.Int).SetUint64(gas)
	energyPrepayed.Mul(energyPrepayed, gasPrice)

	// the checkpoint to be reverted only when gas consumption failure.
	txCheckpoint := rt.state.NewCheckpoint()

	// pre pay energy for tx gas
	addrPayedEnergy, err := rt.consumeEnergy(
		commonTarget,
		energyPrepayed)
	if err != nil {
		rt.state.RevertTo(txCheckpoint)
		return nil, nil, err
	}

	// checkpoint to be reverted when clause failure.
	clauseCheckpoint := rt.state.NewCheckpoint()

	leftOverGas := gas - intrinsicGas

	receipt := Tx.Receipt{Outputs: make([]*Tx.Output, len(clauses))}
	vmOutputs := make([]*vm.Output, len(clauses))

	for i, clause := range clauses {
		vmOutput := rt.Execute(clause, i, leftOverGas)
		vmOutputs[i] = vmOutput

		gasUsed := leftOverGas - vmOutput.LeftOverGas
		leftOverGas = vmOutput.LeftOverGas

		// Apply refund counter, capped to half of the used gas.
		halfUsed := new(big.Int).SetUint64(gasUsed / 2)
		refund := math.BigMin(vmOutput.RefundGas, halfUsed)

		// won't overflow
		leftOverGas += refund.Uint64()

		if vmOutput.VMErr != nil {
			// vm exception here
			// revert all executed clauses
			rt.state.RevertTo(clauseCheckpoint)
			receipt.Outputs = nil
			break
		}

		// transform vm output to clause output
		var logs []*Tx.Log
		for _, vmLog := range vmOutput.Logs {
			logs = append(logs, (*Tx.Log)(vmLog))
		}
		receipt.Outputs[i] = &Tx.Output{Logs: logs}
	}

	receipt.GasUsed = gas - leftOverGas

	// entergy to return = leftover gas * gas price
	energyToReturn := new(big.Int).SetUint64(leftOverGas)
	energyToReturn.Mul(energyToReturn, gasPrice)

	// return overpayed energy to whom payed
	if err := rt.chargeEnergy(addrPayedEnergy, energyToReturn); err != nil {
		rt.state.RevertTo(txCheckpoint)
		return nil, nil, err
	}
	return &receipt, vmOutputs, nil
}

func commonTo(clauses Tx.Clauses) *thor.Address {
	if len(clauses) == 0 {
		return nil
	}
	firstTo := clauses[0].To
	if firstTo == nil {
		return nil
	}

	for _, c := range clauses[1:] {
		if c.To == nil {
			return nil
		}
		if *c.To != *firstTo {
			return nil
		}
	}
	return firstTo
}
