// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/vm/evm"
	"github.com/vechain/thor/xenv"
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
	vmConfig vm.Config
	seeker   *chain.Seeker
	state    *state.State
	ctx      *xenv.BlockContext
}

// New create a Runtime object.
func New(
	seeker *chain.Seeker,
	state *state.State,
	ctx *xenv.BlockContext,
) *Runtime {
	return &Runtime{
		seeker: seeker,
		state:  state,
		ctx:    ctx,
	}
}

func (rt *Runtime) Seeker() *chain.Seeker       { return rt.seeker }
func (rt *Runtime) State() *state.State         { return rt.state }
func (rt *Runtime) Context() *xenv.BlockContext { return rt.ctx }

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
	txCtx *xenv.TransactionContext,
	isStatic bool,
) *vm.Output {
	to := clause.To()
	if isStatic && to == nil {
		panic("static call requires 'To'")
	}
	ctx := vm.Context{
		Beneficiary: rt.ctx.Beneficiary,
		BlockNumber: rt.ctx.Number,
		Time:        rt.ctx.Time,
		GasLimit:    rt.ctx.GasLimit,

		Origin:   txCtx.Origin,
		GasPrice: txCtx.GasPrice,
		TxID:     txCtx.ID,

		GetHash:     rt.seeker.GetID,
		ClauseIndex: index,
		InterceptContractCall: func(evm *evm.EVM, contract *evm.Contract, readonly bool) func() ([]byte, error) {
			return builtin.HandleNativeCall(rt.seeker, rt.state, (*xenv.BlockContext)(rt.ctx), (*xenv.TransactionContext)(txCtx), evm, contract, readonly)
		},
		OnCreateContract: func(evm *evm.EVM, contractAddr thor.Address, caller thor.Address) {
			// set master for created contract
			rt.state.SetMaster(contractAddr, caller)
		},
		OnSuicideContract: func(evm *evm.EVM, contractAddr thor.Address, tokenReceiver thor.Address) {
			amount := rt.state.GetEnergy(contractAddr, rt.ctx.Time)
			if amount.Sign() == 0 {
				return
			}
			// add remained energy of suiciding contract to receiver.
			// no need to clear contract's energy, vm will delete the whole contract later.
			rt.state.SetEnergy(tokenReceiver,
				new(big.Int).Add(
					rt.state.GetEnergy(tokenReceiver, rt.ctx.Time),
					amount),
				rt.ctx.Time)

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
	if to == nil {
		return env.Create(txCtx.Origin, clause.Data(), gas, clause.Value())
	} else if isStatic {
		return env.StaticCall(txCtx.Origin, *to, clause.Data(), gas)
	} else {
		return env.Call(txCtx.Origin, *to, clause.Data(), gas, clause.Value())
	}
}

// Call executes single clause.
func (rt *Runtime) Call(
	clause *Tx.Clause,
	index uint32,
	gas uint64,
	txCtx *xenv.TransactionContext,
) *vm.Output {
	return rt.execute(clause, index, gas, txCtx, false)
}

// ExecuteTransaction executes a transaction.
// If some clause failed, receipt.Outputs will be nil and vmOutputs may shorter than clause count.
func (rt *Runtime) ExecuteTransaction(tx *Tx.Transaction) (receipt *Tx.Receipt, vmOutputs []*vm.Output, err error) {
	resolvedTx, err := ResolveTransaction(rt.state, tx)
	if err != nil {
		return nil, nil, err
	}

	payer, returnGas, err := resolvedTx.BuyGas(rt.state, rt.ctx.Time)
	if err != nil {
		return nil, nil, err
	}

	// ResolveTransaction has checked that tx.Gas() >= IntrinsicGas
	leftOverGas := tx.Gas() - resolvedTx.IntrinsicGas
	// checkpoint to be reverted when clause failure.
	checkpoint := rt.state.NewCheckpoint()

	receipt = &Tx.Receipt{Outputs: make([]*Tx.Output, 0, len(resolvedTx.Clauses))}
	vmOutputs = make([]*vm.Output, 0, len(resolvedTx.Clauses))

	txCtx := resolvedTx.ToContext(rt.ctx.Number, rt.seeker.GetID)
	for i, clause := range resolvedTx.Clauses {
		vmOutput := rt.execute(clause, uint32(i), leftOverGas, txCtx, false)
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
			rt.state.RevertTo(checkpoint)
			receipt.Reverted = true
			receipt.Outputs = nil
			break
		}

		// transform vm output to clause output
		var (
			events    = make(Tx.Events, 0, len(vmOutput.Events))
			transfers = make(Tx.Transfers, 0, len(vmOutput.Transfers))
		)
		for _, vmEvent := range vmOutput.Events {
			events = append(events, (*Tx.Event)(vmEvent))
		}
		for _, vmTransfer := range vmOutput.Transfers {
			transfers = append(transfers, (*Tx.Transfer)(vmTransfer))
		}

		receipt.Outputs = append(receipt.Outputs, &Tx.Output{Events: events, Transfers: transfers})
	}

	receipt.GasUsed = tx.Gas() - leftOverGas
	receipt.GasPayer = payer
	receipt.Paid = new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), resolvedTx.GasPrice)

	returnGas(leftOverGas)

	// reward
	rewardRatio := builtin.Params.Native(rt.state).Get(thor.KeyRewardRatio)
	overallGasPrice := tx.OverallGasPrice(resolvedTx.BaseGasPrice, rt.ctx.Number-1, rt.Seeker().GetID)

	reward := new(big.Int).SetUint64(receipt.GasUsed)
	reward.Mul(reward, overallGasPrice)
	reward.Mul(reward, rewardRatio)
	reward.Div(reward, big.NewInt(1e18))
	builtin.Energy.Native(rt.state).AddBalance(rt.ctx.Beneficiary, reward, rt.ctx.Time)

	receipt.Reward = reward

	return receipt, vmOutputs, nil
}
