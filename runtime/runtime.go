// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime

import (
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime/statedb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	Tx "github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"
)

var (
	energyTransferEvent     *abi.Event
	prototypeSetMasterEvent *abi.Event
	nativeCallReturnGas     uint64 = 1562 // see test case for calculation

	EmptyRuntimeBytecode = []byte{0x60, 0x60, 0x60, 0x40, 0x52, 0x60, 0x02, 0x56}
)

func init() {
	var found bool
	if energyTransferEvent, found = builtin.Energy.ABI.EventByName("Transfer"); !found {
		panic("transfer event not found")
	}
	if prototypeSetMasterEvent, found = builtin.Prototype.Events().EventByName("$Master"); !found {
		panic("$Master event not found")
	}
}

var baseChainConfig = vm.ChainConfig{
	ChainConfig: params.ChainConfig{
		ChainID:             big.NewInt(0),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        big.NewInt(0),
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: nil,
		Ethash:              nil,
		Clique:              nil,
	},
	IstanbulBlock: nil,
}

// Output output of clause execution.
type Output struct {
	Data            []byte
	Events          tx.Events
	Transfers       tx.Transfers
	LeftOverGas     uint64
	RefundGas       uint64
	VMErr           error         // VMErr identify the execution result of the contract function, not evm function's err.
	ContractAddress *thor.Address // if create a new contract, or is nil.
}

type TransactionExecutor struct {
	HasNextClause func() bool
	PrepareNext   func() (exec func() (gasUsed uint64, output *Output, err error), interrupt func())
	Finalize      func() (*tx.Receipt, error)
}

// Runtime bases on EVM and VeChain Thor builtins.
type Runtime struct {
	vmConfig    vm.Config
	chain       *chain.Chain
	state       *state.State
	ctx         *xenv.BlockContext
	chainConfig vm.ChainConfig
}

// New create a Runtime object.
func New(
	chain *chain.Chain,
	state *state.State,
	ctx *xenv.BlockContext,
	forkConfig thor.ForkConfig,
) *Runtime {
	currentChainConfig := baseChainConfig
	currentChainConfig.ConstantinopleBlock = big.NewInt(int64(forkConfig.ETH_CONST))
	currentChainConfig.IstanbulBlock = big.NewInt(int64(forkConfig.ETH_IST))
	if chain != nil {
		// use genesis id as chain id
		currentChainConfig.ChainID = new(big.Int).SetBytes(chain.GenesisID().Bytes())
	}

	// alloc precompiled contracts
	if forkConfig.ETH_IST == ctx.Number {
		for addr := range vm.PrecompiledContractsIstanbul {
			if err := state.SetCode(thor.Address(addr), EmptyRuntimeBytecode); err != nil {
				panic(err)
			}
		}
	} else if ctx.Number == 0 {
		for addr := range vm.PrecompiledContractsByzantium {
			if err := state.SetCode(thor.Address(addr), EmptyRuntimeBytecode); err != nil {
				panic(err)
			}
		}
	}

	// VIP191
	if forkConfig.VIP191 == ctx.Number {
		// upgrade extension contract to V2
		if err := state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes()); err != nil {
			panic(err)
		}
	}

	rt := Runtime{
		chain:       chain,
		state:       state,
		ctx:         ctx,
		chainConfig: currentChainConfig,
	}
	return &rt
}

func (rt *Runtime) Chain() *chain.Chain         { return rt.chain }
func (rt *Runtime) State() *state.State         { return rt.state }
func (rt *Runtime) Context() *xenv.BlockContext { return rt.ctx }

// SetVMConfig config VM.
// Returns this runtime.
func (rt *Runtime) SetVMConfig(config vm.Config) *Runtime {
	rt.vmConfig = config
	return rt
}

func (rt *Runtime) newEVM(stateDB *statedb.StateDB, clauseIndex uint32, txCtx *xenv.TransactionContext) *vm.EVM {
	var lastNonNativeCallGas uint64
	return vm.NewEVM(vm.Context{
		CanTransfer: func(_ vm.StateDB, addr common.Address, amount *big.Int) bool {
			return stateDB.GetBalance(addr).Cmp(amount) >= 0
		},
		Transfer: func(_ vm.StateDB, sender, recipient common.Address, amount *big.Int) {
			if amount.Sign() == 0 {
				return
			}
			// touch energy balance when token balance changed
			// SHOULD be performed before transfer
			senderEnergy, err := rt.state.GetEnergy(thor.Address(sender), rt.ctx.Time)
			if err != nil {
				panic(err)
			}
			recipientEnergy, err := rt.state.GetEnergy(thor.Address(recipient), rt.ctx.Time)
			if err != nil {
				panic(err)
			}

			if err := rt.state.SetEnergy(thor.Address(sender), senderEnergy, rt.ctx.Time); err != nil {
				panic(err)
			}
			if err := rt.state.SetEnergy(thor.Address(recipient), recipientEnergy, rt.ctx.Time); err != nil {
				panic(err)
			}

			stateDB.SubBalance(common.Address(sender), amount)
			stateDB.AddBalance(common.Address(recipient), amount)

			stateDB.AddTransfer(&tx.Transfer{
				Sender:    thor.Address(sender),
				Recipient: thor.Address(recipient),
				Amount:    new(big.Int).Set(amount),
			})
		},
		GetHash: func(num uint64) common.Hash {
			id, err := rt.chain.GetBlockID(uint32(num))
			if err != nil {
				panic(err)
			}
			return common.Hash(id)
		},
		NewContractAddress: func(_ *vm.EVM, counter uint32) common.Address {
			return common.Address(thor.CreateContractAddress(txCtx.ID, clauseIndex, counter))
		},
		InterceptContractCall: func(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error, bool) {
			if evm.Depth() < 2 {
				lastNonNativeCallGas = contract.Gas
				// skip direct calls
				return nil, nil, false
			}

			if contract.Address() != contract.Caller() {
				lastNonNativeCallGas = contract.Gas
				// skip native calls from other contract
				return nil, nil, false
			}

			abi, run, found := builtin.FindNativeCall(thor.Address(contract.Address()), contract.Input)
			if !found {
				lastNonNativeCallGas = contract.Gas
				return nil, nil, false
			}

			if readonly && !abi.Const() {
				panic("invoke non-const method in readonly env")
			}

			if contract.Value().Sign() != 0 {
				// reject value transfer on call
				panic("value transfer not allowed")
			}

			// here we return call gas and extcodeSize gas for native calls, to make
			// builtin contract cheap.
			contract.Gas += nativeCallReturnGas
			if contract.Gas > lastNonNativeCallGas {
				panic("serious bug: native call returned gas over consumed")
			}

			ret, err := xenv.New(abi, rt.chain, rt.state, rt.ctx, txCtx, evm, contract).Call(run)
			return ret, err, true
		},
		OnCreateContract: func(_ *vm.EVM, contractAddr, caller common.Address) {
			// set master for created contract
			if err := rt.state.SetMaster(thor.Address(contractAddr), thor.Address(caller)); err != nil {
				panic(err)
			}

			data, err := prototypeSetMasterEvent.Encode(caller)
			if err != nil {
				panic(err)
			}

			stateDB.AddLog(&types.Log{
				Address: common.Address(contractAddr),
				Topics:  []common.Hash{common.Hash(prototypeSetMasterEvent.ID())},
				Data:    data,
			})
		},
		OnSuicideContract: func(_ *vm.EVM, contractAddr, tokenReceiver common.Address) {
			// it's IMPORTANT to process energy before token
			amount, err := rt.state.GetEnergy(thor.Address(contractAddr), rt.ctx.Time)
			if err != nil {
				panic(err)
			}
			if amount.Sign() != 0 {
				// add remained energy of suiciding contract to receiver.
				// no need to clear contract's energy, vm will delete the whole contract later.
				receiverEnergy, err := rt.state.GetEnergy(thor.Address(tokenReceiver), rt.ctx.Time)
				if err != nil {
					panic(err)
				}
				if err := rt.state.SetEnergy(
					thor.Address(tokenReceiver),
					new(big.Int).Add(receiverEnergy, amount),
					rt.ctx.Time); err != nil {
					panic(err)
				}

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

				stateDB.AddLog(&types.Log{
					Address: common.Address(builtin.Energy.Address),
					Topics:  topics,
					Data:    data,
				})
			}

			if amount := stateDB.GetBalance(contractAddr); amount.Sign() != 0 {
				stateDB.AddBalance(tokenReceiver, amount)

				stateDB.AddTransfer(&tx.Transfer{
					Sender:    thor.Address(contractAddr),
					Recipient: thor.Address(tokenReceiver),
					Amount:    amount,
				})
			}
		},
		Origin:      common.Address(txCtx.Origin),
		GasPrice:    txCtx.GasPrice,
		Coinbase:    common.Address(rt.ctx.Beneficiary),
		GasLimit:    rt.ctx.GasLimit,
		BlockNumber: new(big.Int).SetUint64(uint64(rt.ctx.Number)),
		Time:        new(big.Int).SetUint64(rt.ctx.Time),
		Difficulty:  &big.Int{},
	}, stateDB, &rt.chainConfig, rt.vmConfig)
}

// PrepareClause prepare to execute clause.
// It allows to interrupt execution.
func (rt *Runtime) PrepareClause(
	clause *tx.Clause,
	clauseIndex uint32,
	gas uint64,
	txCtx *xenv.TransactionContext,
) (exec func() (output *Output, interrupted bool, err error), interrupt func()) {
	var (
		stateDB       = statedb.New(rt.state)
		evm           = rt.newEVM(stateDB, clauseIndex, txCtx)
		data          []byte
		leftOverGas   uint64
		vmErr         error
		contractAddr  *thor.Address
		interruptFlag uint32
	)

	exec = func() (output *Output, interrupted bool, err error) {
		defer func() {
			if e := recover(); e != nil {
				// caught state error
				err = e.(error)
			}
		}()

		if clause.To() == nil {
			var caddr common.Address
			data, caddr, leftOverGas, vmErr = evm.Create(vm.AccountRef(txCtx.Origin), clause.Data(), gas, clause.Value())
			contractAddr = (*thor.Address)(&caddr)
		} else {
			data, leftOverGas, vmErr = evm.Call(vm.AccountRef(txCtx.Origin), common.Address(*clause.To()), clause.Data(), gas, clause.Value())
		}

		interrupted = atomic.LoadUint32(&interruptFlag) != 0
		output = &Output{
			Data:            data,
			LeftOverGas:     leftOverGas,
			RefundGas:       stateDB.GetRefund(),
			VMErr:           vmErr,
			ContractAddress: contractAddr,
		}
		output.Events, output.Transfers = stateDB.GetLogs()
		return output, interrupted, nil
	}

	interrupt = func() {
		atomic.StoreUint32(&interruptFlag, 1)
		evm.Cancel()
	}
	return
}

// ExecuteTransaction executes a transaction.
// If some clause failed, receipt.Outputs will be nil and vmOutputs may shorter than clause count.
func (rt *Runtime) ExecuteTransaction(tx *tx.Transaction) (receipt *tx.Receipt, err error) {
	executor, err := rt.PrepareTransaction(tx)
	if err != nil {
		return nil, err
	}
	for executor.HasNextClause() {
		exec, _ := executor.PrepareNext()
		if _, _, err := exec(); err != nil {
			return nil, err
		}
	}
	return executor.Finalize()
}

// PrepareTransaction prepare to execute tx.
func (rt *Runtime) PrepareTransaction(tx *tx.Transaction) (*TransactionExecutor, error) {
	resolvedTx, err := ResolveTransaction(tx)
	if err != nil {
		return nil, err
	}

	baseGasPrice, gasPrice, payer, returnGas, err := resolvedTx.BuyGas(rt.state, rt.ctx.Time)
	if err != nil {
		return nil, err
	}

	txCtx, err := resolvedTx.ToContext(gasPrice, payer, rt.ctx.Number, rt.chain.GetBlockID)
	if err != nil {
		return nil, err
	}

	// ResolveTransaction has checked that tx.Gas() >= IntrinsicGas
	leftOverGas := tx.Gas() - resolvedTx.IntrinsicGas
	// checkpoint to be reverted when clause failure.
	checkpoint := rt.state.NewCheckpoint()

	txOutputs := make([]*Tx.Output, 0, len(resolvedTx.Clauses))
	reverted := false
	finalized := false

	hasNext := func() bool {
		return !reverted && len(txOutputs) < len(resolvedTx.Clauses)
	}

	return &TransactionExecutor{
		HasNextClause: hasNext,
		PrepareNext: func() (exec func() (uint64, *Output, error), interrupt func()) {
			nextClauseIndex := uint32(len(txOutputs))
			execFunc, interrupt := rt.PrepareClause(resolvedTx.Clauses[nextClauseIndex], nextClauseIndex, leftOverGas, txCtx)

			exec = func() (gasUsed uint64, output *Output, err error) {
				if rt.vmConfig.Tracer != nil {
					rt.vmConfig.Tracer.CaptureClauseStart(leftOverGas)
					defer func() {
						rt.vmConfig.Tracer.CaptureClauseEnd(leftOverGas)
					}()
				}

				output, _, err = execFunc()
				if err != nil {
					return 0, nil, err
				}
				gasUsed = leftOverGas - output.LeftOverGas
				leftOverGas = output.LeftOverGas

				// Apply refund counter, capped to half of the used gas.
				refund := gasUsed / 2
				if refund > output.RefundGas {
					refund = output.RefundGas
				}

				// won't overflow
				leftOverGas += refund

				if output.VMErr != nil {
					// vm exception here
					// revert all executed clauses
					rt.state.RevertTo(checkpoint)
					reverted = true
					txOutputs = nil
					return
				}
				txOutputs = append(txOutputs, &Tx.Output{Events: output.Events, Transfers: output.Transfers})
				return
			}

			return
		},
		Finalize: func() (*Tx.Receipt, error) {
			if hasNext() {
				return nil, errors.New("not all clauses processed")
			}
			if finalized {
				return nil, errors.New("already finalized")
			}
			finalized = true

			receipt := &Tx.Receipt{
				Reverted: reverted,
				Outputs:  txOutputs,
				GasUsed:  tx.Gas() - leftOverGas,
				GasPayer: payer,
			}

			receipt.Paid = new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), gasPrice)

			if err := returnGas(leftOverGas); err != nil {
				return nil, err
			}

			// reward
			rewardRatio, err := builtin.Params.Native(rt.state).Get(thor.KeyRewardRatio)
			if err != nil {
				return nil, err
			}
			provedWork, err := tx.ProvedWork(rt.ctx.Number-1, rt.chain.GetBlockID)
			if err != nil {
				return nil, err
			}
			overallGasPrice := tx.OverallGasPrice(baseGasPrice, provedWork)

			reward := new(big.Int).SetUint64(receipt.GasUsed)
			reward.Mul(reward, overallGasPrice)
			reward.Mul(reward, rewardRatio)
			reward.Div(reward, big.NewInt(1e18))
			if err := builtin.Energy.Native(rt.state, rt.ctx.Time).Add(rt.ctx.Beneficiary, reward); err != nil {
				return nil, err
			}

			receipt.Reward = reward
			return receipt, nil
		},
	}, nil
}
