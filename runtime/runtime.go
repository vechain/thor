package runtime

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

var (
	callGas uint64 = 1000 * 1000
)

// Runtime is to support transaction execution.
type Runtime struct {
	vmConfig vm.Config
	getHash  func(uint32) thor.Hash
	state    State

	// block env
	blockBeneficiary thor.Address
	blockNumber      uint32
	blockTime        uint64
	blockGasLimit    uint64
}

// State to decouple state.State.
type State interface {
	vm.State
	RevertTo(revision int)
}

// New create a Runtime object.
func New(
	state State,
	blockBeneficiary thor.Address,
	blockNumber uint32,
	blockTime,
	blockGasLimit uint64,
	getHash func(uint32) thor.Hash,
) *Runtime {
	return &Runtime{
		getHash:          getHash,
		state:            state,
		blockBeneficiary: blockBeneficiary,
		blockNumber:      blockNumber,
		blockTime:        blockTime,
		blockGasLimit:    blockGasLimit,
	}
}

// SetVMConfig config VM.
// Returns this runtime.
func (rt *Runtime) SetVMConfig(config vm.Config) *Runtime {
	rt.vmConfig = config
	return rt
}

// Execute executes single clause bases on tx env set by SetTransactionEnvironment.
func (rt *Runtime) Execute(
	clause *Tx.Clause,
	index int,
	gas uint64,
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txHash thor.Hash,
) *vm.Output {

	ctx := vm.Context{
		Beneficiary: rt.blockBeneficiary,
		BlockNumber: new(big.Int).SetUint64(uint64(rt.blockNumber)),
		Time:        new(big.Int).SetUint64(rt.blockTime),
		GasLimit:    new(big.Int).SetUint64(rt.blockGasLimit),

		Origin:   txOrigin,
		GasPrice: txGasPrice,
		TxHash:   txHash,

		GetHash:     rt.getHash,
		ClauseIndex: uint64(index),
	}

	vm := vm.New(ctx, rt.state, rt.vmConfig)
	to := clause.To()
	if to == nil {
		return vm.Create(txOrigin, clause.Data(), gas, clause.Value())
	}
	return vm.Call(txOrigin, *to, clause.Data(), gas, clause.Value())
}

func (rt *Runtime) consumeEnergy(caller thor.Address, callee thor.Address, amount *big.Int) (thor.Address, error) {
	data := contracts.Energy.PackConsume(caller, callee, amount)
	output := rt.Execute(
		Tx.NewClause(&contracts.Energy.Address).WithData(data),
		0, callGas, contracts.Energy.Address, &big.Int{}, thor.Hash{})
	if output.VMErr != nil {
		return thor.Address{}, errors.Wrap(output.VMErr, "consume energy")
	}

	return contracts.Energy.UnpackConsume(output.Value), nil
}

func (rt *Runtime) chargeEnergy(addr thor.Address, amount *big.Int) error {

	data := contracts.Energy.PackCharge(addr, amount)

	output := rt.Execute(
		Tx.NewClause(&contracts.Energy.Address).WithData(data),
		0, callGas, contracts.Energy.Address, &big.Int{}, thor.Hash{})
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "charge energy")
	}
	return nil
}

// ExecuteTransaction executes a transaction.
// Note that the elements of returned []*vm.Output may be nil if corresponded clause failed.
func (rt *Runtime) ExecuteTransaction(tx *Tx.Transaction, signing *cry.Signing) (receipt *Tx.Receipt, vmOutputs []*vm.Output, err error) {
	// precheck
	origin, err := signing.Signer(tx)
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

	gasPrice := tx.GasPrice()
	clauses := tx.Clauses()

	energyPrepayed := new(big.Int).SetUint64(gas)
	energyPrepayed.Mul(energyPrepayed, gasPrice)

	// pre pay energy for tx gas
	energyPayer, err := rt.consumeEnergy(
		origin,
		commonTo(clauses),
		energyPrepayed)

	if err != nil {
		return nil, nil, err
	}

	// checkpoint to be reverted when clause failure.
	clauseCheckpoint := rt.state.NewCheckpoint()

	leftOverGas := gas - intrinsicGas

	receipt = &Tx.Receipt{Outputs: make([]*Tx.Output, len(clauses))}
	vmOutputs = make([]*vm.Output, len(clauses))

	for i, clause := range clauses {
		vmOutput := rt.Execute(clause, i, leftOverGas, origin, gasPrice, tx.Hash())
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
	if err := rt.chargeEnergy(energyPayer, energyToReturn); err != nil {
		return nil, nil, err
	}
	return receipt, vmOutputs, nil
}

// returns common 'To' field of clauses if any.
// Empty address returned if no common 'To'.
func commonTo(clauses []*Tx.Clause) thor.Address {
	if len(clauses) == 0 {
		return thor.Address{}
	}

	firstTo := clauses[0].To()
	if firstTo == nil {
		return thor.Address{}
	}

	for _, clause := range clauses[1:] {
		to := clause.To()
		if to == nil {
			return thor.Address{}
		}
		if *to != *firstTo {
			return thor.Address{}
		}
	}
	return *firstTo
}
