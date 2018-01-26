package runtime

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

// Runtime is to support transaction execution.
type Runtime struct {
	vmConfig   vm.Config
	getBlockID func(uint32) thor.Hash
	state      *state.State

	// block env
	blockBeneficiary thor.Address
	blockNumber      uint32
	blockTime        uint64
	blockGasLimit    uint64
}

// New create a Runtime object.
func New(
	state *state.State,
	blockBeneficiary thor.Address,
	blockNumber uint32,
	blockTime,
	blockGasLimit uint64,
	getBlockID func(uint32) thor.Hash) *Runtime {
	return &Runtime{
		getBlockID:       getBlockID,
		state:            state,
		blockBeneficiary: blockBeneficiary,
		blockNumber:      blockNumber,
		blockTime:        blockTime,
		blockGasLimit:    blockGasLimit,
	}
}

func (rt *Runtime) State() *state.State            { return rt.state }
func (rt *Runtime) BlockBeneficiary() thor.Address { return rt.blockBeneficiary }
func (rt *Runtime) BlockNumber() uint32            { return rt.blockNumber }
func (rt *Runtime) BlockTime() uint64              { return rt.blockTime }
func (rt *Runtime) BlockGasLimit() uint64          { return rt.blockGasLimit }

// SetVMConfig config VM.
// Returns this runtime.
func (rt *Runtime) SetVMConfig(config vm.Config) *Runtime {
	rt.vmConfig = config
	return rt
}

func (rt *Runtime) execute(
	clause *Tx.Clause,
	index int,
	gas uint64,
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txID thor.Hash,
	isStatic bool,
) *vm.Output {
	to := clause.To()
	if isStatic && to == nil {
		panic("static call requires 'To'")
	}
	ctx := vm.Context{
		Beneficiary: rt.blockBeneficiary,
		BlockNumber: new(big.Int).SetUint64(uint64(rt.blockNumber)),
		Time:        new(big.Int).SetUint64(rt.blockTime),
		GasLimit:    new(big.Int).SetUint64(rt.blockGasLimit),

		Origin:   txOrigin,
		GasPrice: txGasPrice,
		TxHash:   txID,

		GetHash:     rt.getBlockID,
		ClauseIndex: uint64(index),
	}

	vm := vm.New(ctx, rt.state, rt.vmConfig)
	if to == nil {
		return vm.Create(txOrigin, clause.Data(), gas, clause.Value())
	}
	if isStatic {
		return vm.StaticCall(txOrigin, *to, clause.Data(), gas)
	}
	return vm.Call(txOrigin, *to, clause.Data(), gas, clause.Value())
}

// StaticCall executes signle clause which ensure no modifications to state.
func (rt *Runtime) StaticCall(
	clause *Tx.Clause,
	index int,
	gas uint64,
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txID thor.Hash,
) *vm.Output {
	return rt.execute(clause, index, gas, txOrigin, txGasPrice, txID, true)
}

// Call executes single clause.
func (rt *Runtime) Call(
	clause *Tx.Clause,
	index int,
	gas uint64,
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txID thor.Hash,
) *vm.Output {
	return rt.execute(clause, index, gas, txOrigin, txGasPrice, txID, false)
}

func (rt *Runtime) consumeEnergy(caller thor.Address, callee thor.Address, amount *big.Int) (thor.Address, error) {
	clause := contracts.Energy.PackConsume(caller, callee, amount)
	output := rt.execute(clause,
		0, math.MaxUint32, contracts.Energy.Address, &big.Int{}, thor.Hash{}, false)
	if output.VMErr != nil {
		return thor.Address{}, errors.Wrap(output.VMErr, "consume energy")
	}

	return contracts.Energy.UnpackConsume(output.Value), nil
}

func (rt *Runtime) chargeEnergy(addr thor.Address, amount *big.Int) error {
	clause := contracts.Energy.PackCharge(addr, amount)
	output := rt.execute(clause,
		0, math.MaxUint32, contracts.Energy.Address, &big.Int{}, thor.Hash{}, false)
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "charge energy")
	}
	return nil
}

// ExecuteTransaction executes a transaction.
// Note that the elements of returned []*vm.Output may be nil if corresponded clause failed.
func (rt *Runtime) ExecuteTransaction(tx *Tx.Transaction) (receipt *Tx.Receipt, vmOutputs []*vm.Output, err error) {
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
		vmOutput := rt.execute(clause, i, leftOverGas, origin, gasPrice, tx.ID(), false)
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
