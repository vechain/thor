package runtime

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/vechain/thor/builtin"
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
	index uint32,
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
		BlockNumber: rt.blockNumber,
		Time:        rt.blockTime,
		GasLimit:    rt.blockGasLimit,

		Origin:   txOrigin,
		GasPrice: txGasPrice,
		TxID:     txID,

		GetHash:     rt.getBlockID,
		ClauseIndex: index,
	}

	env := vm.New(ctx, rt.state, rt.vmConfig)
	env.SetContractHook(func(to thor.Address, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
		return builtin.HandleNativeCall(rt.state, &ctx, to, input)
	})
	env.SetOnContractCreated(func(contractAddr thor.Address) {
		// set master for created contract
		builtin.Energy.SetContractMaster(rt.state, contractAddr, txOrigin)
	})
	env.SetOnTransfer(func(sender, recipient thor.Address, amount *big.Int) {
		if amount.Sign() == 0 {
			return
		}
		// touch energy accounts which token balance changed
		builtin.Energy.AddBalance(rt.state, rt.blockTime, sender, &big.Int{})
		builtin.Energy.AddBalance(rt.state, rt.blockTime, recipient, &big.Int{})
	})

	if to == nil {
		return env.Create(txOrigin, clause.Data(), gas, clause.Value())
	}
	if isStatic {
		return env.StaticCall(txOrigin, *to, clause.Data(), gas)
	}
	return env.Call(txOrigin, *to, clause.Data(), gas, clause.Value())
}

// StaticCall executes signle clause which ensure no modifications to state.
func (rt *Runtime) StaticCall(
	clause *Tx.Clause,
	index uint32,
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
	index uint32,
	gas uint64,
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txID thor.Hash,
) *vm.Output {
	return rt.execute(clause, index, gas, txOrigin, txGasPrice, txID, false)
}

// ExecuteTransaction executes a transaction.
// If some clause failed, receipt.Outputs will be nil and vmOutputs may shorter than clause count.
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

	energyPayer, ok := builtin.Energy.Consume(rt.state, rt.blockTime, origin, commonTo(clauses), energyPrepayed)
	if !ok {
		return nil, nil, errors.New("insufficient energy")
	}

	// checkpoint to be reverted when clause failure.
	clauseCheckpoint := rt.state.NewCheckpoint()

	leftOverGas := gas - intrinsicGas

	receipt = &Tx.Receipt{Outputs: make([]*Tx.Output, 0, len(clauses))}
	vmOutputs = make([]*vm.Output, 0, len(clauses))

	for i, clause := range clauses {
		vmOutput := rt.execute(clause, uint32(i), leftOverGas, origin, gasPrice, tx.ID(), false)
		vmOutputs = append(vmOutputs, vmOutput)

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
		receipt.Outputs = append(receipt.Outputs, &Tx.Output{Logs: logs})
	}

	receipt.GasUsed = gas - leftOverGas

	// entergy to return = leftover gas * gas price
	energyToReturn := new(big.Int).SetUint64(leftOverGas)
	energyToReturn.Mul(energyToReturn, gasPrice)

	// return overpayed energy to payer
	builtin.Energy.AddBalance(rt.state, rt.blockTime, energyPayer, energyToReturn)

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
