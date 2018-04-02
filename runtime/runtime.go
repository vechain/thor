package runtime

import (
	"math/big"

	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/builtin/energy"
	"github.com/vechain/thor/builtin/params"
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

	energy *energy.Energy
	params *params.Params
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
		energy:           builtin.Energy.WithState(state),
		params:           builtin.Params.WithState(state),
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
		rt.energy.SetContractMaster(contractAddr, txOrigin)
	})
	env.SetOnTransfer(func(sender, recipient thor.Address, amount *big.Int) {
		if amount.Sign() == 0 {
			return
		}
		// touch energy accounts which token balance changed
		rt.energy.AddBalance(rt.blockTime, sender, &big.Int{})
		rt.energy.AddBalance(rt.blockTime, recipient, &big.Int{})
	})

	if to == nil {
		return env.Create(txOrigin, clause.Data(), gas, clause.Value())
	}
	if isStatic {
		return env.StaticCall(txOrigin, *to, clause.Data(), gas)
	}
	return env.Call(txOrigin, *to, clause.Data(), gas, clause.Value())
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
	resolvedTx, err := ResolveTransaction(rt.state, tx)
	if err != nil {
		return nil, nil, err
	}

	payer, err := resolvedTx.BuyGas(rt.blockTime)
	if err != nil {
		return nil, nil, err
	}

	// checkpoint to be reverted when clause failure.
	clauseCheckpoint := rt.state.NewCheckpoint()

	leftOverGas := tx.Gas() - resolvedTx.IntrinsicGas

	receipt = &Tx.Receipt{Outputs: make([]*Tx.Output, 0, len(resolvedTx.Clauses))}
	vmOutputs = make([]*vm.Output, 0, len(resolvedTx.Clauses))

	for i, clause := range resolvedTx.Clauses {
		vmOutput := rt.execute(clause, uint32(i), leftOverGas, resolvedTx.Origin, resolvedTx.GasPrice, tx.ID(), false)
		vmOutputs = append(vmOutputs, vmOutput)

		gasUsed := leftOverGas - vmOutput.LeftOverGas
		leftOverGas = vmOutput.LeftOverGas

		// Apply refund counter, capped to half of the used gas.
		refund := gasUsed / 2
		if refund > vmOutput.RefundGas {
			refund = vmOutput.RefundGas
		}

		// won't overflow
		leftOverGas += refund

		if vmOutput.VMErr != nil {
			// vm exception here
			// revert all executed clauses
			rt.state.RevertTo(clauseCheckpoint)
			receipt.Reverted = true
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

	receipt.GasUsed = tx.Gas() - leftOverGas
	receipt.GasPayer = payer

	// entergy to return = leftover gas * gas price
	energyToReturn := new(big.Int).Mul(new(big.Int).SetUint64(leftOverGas), resolvedTx.GasPrice)

	// return overpayed energy to payer
	rt.energy.AddBalance(rt.blockTime, payer, energyToReturn)

	// reward
	rewardRatio := rt.params.Get(thor.KeyRewardRatio)
	overallGasPrice := tx.OverallGasPrice(resolvedTx.BaseGasPrice, rt.blockNumber-1, rt.getBlockID)
	reward := new(big.Int).SetUint64(receipt.GasUsed)
	reward.Mul(reward, overallGasPrice)
	reward.Mul(reward, rewardRatio)
	reward.Div(reward, big.NewInt(1e18))
	rt.energy.AddBalance(rt.blockTime, rt.blockBeneficiary, reward)

	receipt.Reward = reward

	return receipt, vmOutputs, nil
}
