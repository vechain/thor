package runtime

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
)

var energyTransferEvent *abi.Event

func init() {
	ev, found := builtin.Energy.ABI.EventByName("Transfer")
	if !found {
		panic("transfer event not found")
	}
	energyTransferEvent = ev
}

// Runtime is to support transaction execution.
type Runtime struct {
	vmConfig   vm.Config
	getBlockID func(uint32) thor.Bytes32
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
	getBlockID func(uint32) thor.Bytes32) *Runtime {
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
	txID thor.Bytes32,
	isStatic bool,
) (*vm.Output, Tx.Transfers) {
	to := clause.To()
	if isStatic && to == nil {
		panic("static call requires 'To'")
	}
	var transfers Tx.Transfers
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
		OnTransfer: func(depth int, sender, recipient thor.Address, amount *big.Int) {
			if depth == 0 || amount.Sign() != 0 {
				transfers = append(transfers, &Tx.Transfer{
					Sender:    sender,
					Recipient: recipient,
					Amount:    amount})
			}

			if amount.Sign() == 0 {
				return
			}
			// touch energy accounts which token balance changed
			builtin.Energy.Native(rt.state).AddBalance(sender, &big.Int{}, rt.blockNumber)
			builtin.Energy.Native(rt.state).AddBalance(recipient, &big.Int{}, rt.blockNumber)
		},
		ContractHook: func(evm *evm.EVM, contract *evm.Contract, readonly bool) func() ([]byte, error) {
			return builtin.HandleNativeCall(rt.state, evm, contract, readonly)
		},
		OnCreateContract: func(evm *evm.EVM, contractAddr thor.Address, caller thor.Address) {
			// set master for created contract
			builtin.Prototype.Native(rt.state).Bind(contractAddr).SetMaster(caller)
		},
		OnSuicideContract: func(evm *evm.EVM, contractAddr thor.Address, tokenReceiver thor.Address) {
			amount := rt.state.GetEnergy(contractAddr, rt.blockNumber)
			if amount.Sign() == 0 {
				return
			}
			// add remained energy of suiciding contract to receiver.
			// no need to clear contract's energy, vm will delete the whole contract later.
			rt.state.SetEnergy(tokenReceiver,
				new(big.Int).Add(
					rt.state.GetEnergy(tokenReceiver, rt.blockNumber),
					amount),
				rt.blockNumber)

			// see ERC20's Transfer event
			topics := []common.Hash{
				common.Hash(energyTransferEvent.ID()),
				common.BytesToHash(contractAddr[:]),
				common.BytesToHash(tokenReceiver[:]),
			}

			data, err := energyTransferEvent.Encode(amount)
			if err != nil {
				panic(err)
			}

			evm.StateDB.AddLog(&types.Log{
				Address: common.Address(builtin.Energy.Address),
				Topics:  topics,
				Data:    data,
			})
		},
	}

	env := vm.New(ctx, rt.state, rt.vmConfig)
	var vmout *vm.Output
	if to == nil {
		vmout = env.Create(txOrigin, clause.Data(), gas, clause.Value())
	} else if isStatic {
		vmout = env.StaticCall(txOrigin, *to, clause.Data(), gas)
	} else {
		vmout = env.Call(txOrigin, *to, clause.Data(), gas, clause.Value())
	}
	if vmout.VMErr != nil {
		return vmout, nil
	}
	return vmout, transfers
}

// Call executes single clause.
func (rt *Runtime) Call(
	clause *Tx.Clause,
	index uint32,
	gas uint64,
	txOrigin thor.Address,
	txGasPrice *big.Int,
	txID thor.Bytes32,
) (*vm.Output, Tx.Transfers) {
	return rt.execute(clause, index, gas, txOrigin, txGasPrice, txID, false)
}

// ExecuteTransaction executes a transaction.
// If some clause failed, receipt.Outputs will be nil and vmOutputs may shorter than clause count.
func (rt *Runtime) ExecuteTransaction(tx *Tx.Transaction) (receipt *Tx.Receipt, txTransfers []Tx.Transfers, vmOutputs []*vm.Output, err error) {
	resolvedTx, err := ResolveTransaction(rt.state, tx)
	if err != nil {
		return nil, nil, nil, err
	}

	payer, _, returnGas, err := resolvedTx.BuyGas(rt.blockNumber)
	if err != nil {
		return nil, nil, nil, err
	}

	// checkpoint to be reverted when clause failure.
	clauseCheckpoint := rt.state.NewCheckpoint()

	leftOverGas := tx.Gas() - resolvedTx.IntrinsicGas

	receipt = &Tx.Receipt{Outputs: make([]*Tx.Output, 0, len(resolvedTx.Clauses))}
	vmOutputs = make([]*vm.Output, 0, len(resolvedTx.Clauses))
	txTransfers = make([]Tx.Transfers, 0, len(resolvedTx.Clauses))

	for i, clause := range resolvedTx.Clauses {
		vmOutput, transfers := rt.execute(clause, uint32(i), leftOverGas, resolvedTx.Origin, resolvedTx.GasPrice, tx.ID(), false)
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
			txTransfers = nil
			break
		}

		// transform vm output to clause output
		var events Tx.Events
		for _, vmLog := range vmOutput.Logs {
			events = append(events, (*Tx.Event)(vmLog))
		}
		receipt.Outputs = append(receipt.Outputs, &Tx.Output{Events: events})
		txTransfers = append(txTransfers, transfers)
	}

	receipt.GasUsed = tx.Gas() - leftOverGas
	receipt.GasPayer = payer
	receipt.Paid = new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), resolvedTx.GasPrice)

	returnGas(leftOverGas)

	// reward
	rewardRatio := builtin.Params.Native(rt.state).Get(thor.KeyRewardRatio)
	overallGasPrice := tx.OverallGasPrice(resolvedTx.BaseGasPrice, rt.blockNumber-1, rt.getBlockID)
	reward := new(big.Int).SetUint64(receipt.GasUsed)
	reward.Mul(reward, overallGasPrice)
	reward.Mul(reward, rewardRatio)
	reward.Div(reward, big.NewInt(1e18))
	builtin.Energy.Native(rt.state).AddBalance(rt.blockBeneficiary, reward, rt.blockNumber)

	receipt.Reward = reward

	return receipt, txTransfers, vmOutputs, nil
}
