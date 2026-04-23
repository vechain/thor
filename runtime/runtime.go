// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/runtime/statedb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

// runtimeStateDB is the superset of vm.StateDB used by the runtime: it adds
// the Thor-specific transfer / log collection helpers. Both *statedb.StateDB
// (V1) and *statedb.StateDBV2 satisfy it, so `newEVM` can accept either and
// the VM's interface-dispatched SetNonce routes to whichever was selected.
type runtimeStateDB interface {
	vm.StateDB
	AddTransfer(*tx.Transfer)
	GetLogs() (tx.Events, tx.Transfers)
}

var (
	energyTransferEvent     *abi.Event
	prototypeSetMasterEvent *abi.Event
	nativeCallReturnGas     uint64 = 1562 // see test case for calculation

	// EmptyRuntimeBytecode is stored at every precompile address at fork activation.
	// This makes precompile addresses "exist" in Thor's state, which prevents
	// accidental contract deployment to those addresses (CREATE/CREATE2 will fail
	// with ErrContractAddressCollision since the code hash differs from emptyCodeHash).
	//
	// NOTE: This means EXTCODESIZE/EXTCODECOPY/EXTCODEHASH on any precompile address
	// returns 8 bytes of non-empty code on Thor, whereas on Ethereum precompile
	// addresses have no code (EXTCODESIZE returns 0). Contracts that detect
	// precompiles by checking extcodesize == 0 will behave differently on Thor.
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
	ShanghaiBlock: nil,
	OsakaBlock:    nil,
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
	PrepareNext   func() (exec func() (interrupted bool, err error), interrupt func())
	Finalize      func() (*tx.Receipt, error)
}

// Runtime bases on EVM and VeChain Thor builtins.
type Runtime struct {
	vmConfig    vm.Config
	chain       *chain.Chain
	state       *state.State
	ctx         *xenv.BlockContext
	chainConfig vm.ChainConfig
	forkConfig  *thor.ForkConfig
}

// New create a Runtime object.
func New(
	chain *chain.Chain,
	state *state.State,
	ctx *xenv.BlockContext,
	forkConfig *thor.ForkConfig,
) *Runtime {
	currentChainConfig := baseChainConfig
	currentChainConfig.ConstantinopleBlock = big.NewInt(int64(forkConfig.ETH_CONST))
	currentChainConfig.IstanbulBlock = big.NewInt(int64(forkConfig.ETH_IST))
	currentChainConfig.ShanghaiBlock = big.NewInt(int64(forkConfig.GALACTICA))
	currentChainConfig.OsakaBlock = big.NewInt(int64(forkConfig.INTERSTELLAR))
	if chain != nil {
		if thor.IsForked(ctx.Number, forkConfig.INTERSTELLAR) {
			// Post-INTERSTELLAR: 2-byte EIP-155 CHAIN_ID from the last two bytes of genesis id.
			currentChainConfig.ChainID = new(big.Int).SetUint64(thor.ChainID(chain.GenesisID()))
		} else {
			// Pre-INTERSTELLAR: legacy 32-byte genesis id interpreted as uint256.
			currentChainConfig.ChainID = new(big.Int).SetBytes(chain.GenesisID().Bytes())
		}
	}

	// allocate precompiled contracts
	var precompiled map[common.Address]vm.PrecompiledContract
	if forkConfig.INTERSTELLAR == ctx.Number {
		precompiled = vm.PrecompiledContractsOsaka
	} else if forkConfig.GALACTICA == ctx.Number {
		precompiled = vm.PrecompiledContractsShanghai
	} else if forkConfig.ETH_IST == ctx.Number {
		precompiled = vm.PrecompiledContractsIstanbul
	} else if ctx.Number == 0 {
		precompiled = vm.PrecompiledContractsByzantium
	}
	for addr := range precompiled {
		if err := state.SetCode(thor.Address(addr), EmptyRuntimeBytecode); err != nil {
			panic(err)
		}
	}

	// set builtin contracts
	if forkConfig.GALACTICA == ctx.Number {
		if err := state.SetCode(builtin.Extension.Address, builtin.Extension.V3.RuntimeBytecodes()); err != nil {
			panic(err)
		}
	} else if forkConfig.VIP191 == ctx.Number {
		if err := state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes()); err != nil {
			panic(err)
		}
	}

	// Prepare the transition period
	if forkConfig.HAYABUSA == ctx.Number {
		if err := state.SetCode(builtin.Staker.Address, builtin.Staker.RuntimeBytecodes()); err != nil {
			panic(err)
		}
		if err := builtin.Energy.Native(state, ctx.Time).StopEnergyGrowth(); err != nil {
			panic(err)
		}
	}

	rt := Runtime{
		chain:       chain,
		state:       state,
		ctx:         ctx,
		chainConfig: currentChainConfig,
		forkConfig:  forkConfig,
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

// newStateDB returns the StateDB flavor appropriate for this tx + fork.
// Post-INTERSTELLAR 0x02 (TypeEthDynamicFee) gets V2 whose SetNonce writes
// through to account state so sender-nonce bumps and EIP-158 contract-nonce
// initialization persist. Everything else (0x00 / 0x51, any pre-INT) gets
// V1 whose SetNonce is a no-op so `SetNonce` calls from shared code paths
// are semantically harmless. This centralizes the tx-type + fork gate in a
// single helper (spec 3 §2.1 / §3.3).
func (rt *Runtime) newStateDB(trx *tx.Transaction) runtimeStateDB {
	if trx != nil &&
		trx.Type() == tx.TypeEthDynamicFee &&
		thor.IsForked(rt.ctx.Number, rt.forkConfig.INTERSTELLAR) {
		return statedb.NewV2(rt.state)
	}
	return statedb.New(rt.state)
}

func (rt *Runtime) newEVM(stateDB runtimeStateDB, clauseIndex uint32, txCtx *xenv.TransactionContext, trx *tx.Transaction) *vm.EVM {
	var (
		lastNonNativeCallGas uint64
		baseFee              *big.Int
	)
	if rt.ctx.BaseFee != nil {
		baseFee = new(big.Int).Set(rt.ctx.BaseFee)
	}
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
			senderEnergy, err := builtin.Energy.Native(rt.state, rt.ctx.Time).Get(thor.Address(sender))
			if err != nil {
				panic(err)
			}
			recipientEnergy, err := builtin.Energy.Native(rt.state, rt.ctx.Time).Get(thor.Address(recipient))
			if err != nil {
				panic(err)
			}

			if err := rt.state.SetEnergy(thor.Address(sender), senderEnergy, rt.ctx.Time); err != nil {
				panic(err)
			}
			if err := rt.state.SetEnergy(thor.Address(recipient), recipientEnergy, rt.ctx.Time); err != nil {
				panic(err)
			}

			stateDB.SubBalance(sender, amount)
			stateDB.AddBalance(recipient, amount)

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
		NewContractAddress: func(evm *vm.EVM, counter uint32, caller common.Address) common.Address {
			// Spec 3 §2.3: for 0x02 txs past INTERSTELLAR, produce eth-style
			// `keccak(rlp(sender, nonce))[12:]` CREATE addresses; all other tx
			// types keep VeChain-native derivation. Dispatch happens here
			// (runtime side) rather than inside vm/evm.go so the VM stays
			// chain-agnostic.
			if trx != nil &&
				trx.Type() == tx.TypeEthDynamicFee &&
				thor.IsForked(rt.ctx.Number, rt.forkConfig.INTERSTELLAR) {
				nonce := evm.StateDB.GetNonce(caller)
				if evm.Depth() == 0 && caller == common.Address(txCtx.Origin) {
					// Top-level CREATE from origin: runtime pre-bumped origin
					// to N+1 outside the checkpoint (spec 3 §2.1). The VM's
					// own sender-nonce bump at vm/evm.go:461 is gated by
					// depth>0, so the caller.Nonce we read here is N+1; the
					// eth address formula wants the pre-bump value (N) =
					// nonce - 1.
					return common.Address(crypto.CreateAddress(caller, nonce-1))
				}
				// Inner CREATE (or any non-origin top-level, which shouldn't
				// happen in practice): caller is a contract whose nonce will
				// be bumped by the VM after this callback runs. The eth
				// formula wants the pre-bump value = current read.
				return common.Address(crypto.CreateAddress(caller, nonce))
			}
			// 0x00 / 0x51 / pre-INT — VeChain-native derivation unchanged.
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

			abi, run, found, returnsGas := builtin.FindNativeCall(thor.Address(contract.Address()), contract.Input)
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

			if returnsGas {
				// here we return call gas and extcodeSize gas for native calls, to make
				// builtin contract cheap, this only applies to the 0.4.24 complied contracts
				contract.Gas += nativeCallReturnGas
				if contract.Gas > lastNonNativeCallGas {
					panic("serious bug: native call returned gas over consumed")
				}
			}

			ret, err := xenv.New(abi, rt.chain, rt.state, rt.ctx, rt.forkConfig, txCtx, evm, contract, clauseIndex).Call(run)
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
				Address: contractAddr,
				Topics:  []common.Hash{common.Hash(prototypeSetMasterEvent.ID())},
				Data:    data,
			})
		},
		// shouldDestroy indicates if the contract will be destroyed in the current execution, introduced by EIP6780
		OnSuicideContract: func(evm *vm.EVM, contractAddr, tokenReceiver common.Address, shouldDestroy bool) {
			// it's IMPORTANT to process energy before token
			energy, err := builtin.Energy.Native(rt.state, rt.ctx.Time).Get(thor.Address(contractAddr))
			if err != nil {
				panic(err)
			}
			bal := stateDB.GetBalance(contractAddr)
			toSelf := contractAddr == tokenReceiver

			if energy.Sign() != 0 {
				// take care of the energy transfer for both contract and the receiver, skip if transfer to self
				if !toSelf {
					receiverEnergy, err := builtin.Energy.Native(rt.state, rt.ctx.Time).Get(thor.Address(tokenReceiver))
					if err != nil {
						panic(err)
					}
					if err := rt.state.SetEnergy(
						thor.Address(tokenReceiver),
						new(big.Int).Add(receiverEnergy, energy),
						rt.ctx.Time); err != nil {
						panic(err)
					}
					if err := rt.state.SetEnergy(
						thor.Address(contractAddr),
						big.NewInt(0),
						rt.ctx.Time,
					); err != nil {
						panic(err)
					}
				}
				// see ERC20's Transfer event
				topics := []common.Hash{
					common.Hash(energyTransferEvent.ID()),
					common.BytesToHash(contractAddr[:]),
					common.BytesToHash(tokenReceiver[:]),
				}
				data, err := energyTransferEvent.Encode(energy)
				if err != nil {
					panic(err)
				}

				// do not emit log if transfer to self and shouldDestroy is false
				if !toSelf || shouldDestroy {
					stateDB.AddLog(&types.Log{
						Address: common.Address(builtin.Energy.Address),
						Topics:  topics,
						Data:    data,
					})
				}
			}

			if bal.Sign() != 0 {
				// take care of the balance transfer for both contract and the receiver, skip if transfer to self
				if !toSelf {
					stateDB.AddBalance(tokenReceiver, bal)
					stateDB.SubBalance(contractAddr, bal)
				}
				// do not emit log if transfer to self and shouldDestroy is false
				if !toSelf || shouldDestroy {
					stateDB.AddTransfer(&tx.Transfer{
						Sender:    thor.Address(contractAddr),
						Recipient: thor.Address(tokenReceiver),
						Amount:    bal,
					})
				}
			}
		},

		Origin:      common.Address(txCtx.Origin),
		GasPrice:    txCtx.GasPrice,
		Coinbase:    common.Address(rt.ctx.Beneficiary),
		GasLimit:    rt.ctx.GasLimit,
		BlockNumber: new(big.Int).SetUint64(uint64(rt.ctx.Number)),
		Time:        new(big.Int).SetUint64(rt.ctx.Time),
		Difficulty:  &big.Int{},
		BaseFee:     baseFee,
	}, stateDB, &rt.chainConfig, rt.vmConfig)
}

// PrepareClause prepare to execute clause.
// It allows to interrupt execution.
//
// Callers with no transaction context (genesis builder, runtime unit tests)
// reach here directly; PrepareTransaction threads the full tx through
// prepareClauseWithTx instead so V1/V2 StateDB selection and eth-style CREATE
// derivation can kick in when the tx is 0x02 post-INTERSTELLAR.
func (rt *Runtime) PrepareClause(
	clause *tx.Clause,
	clauseIndex uint32,
	gas uint64,
	txCtx *xenv.TransactionContext,
) (exec func() (output *Output, interrupted bool, err error), interrupt func()) {
	return rt.prepareClauseWithTx(clause, clauseIndex, gas, txCtx, nil)
}

// prepareClauseWithTx is the internal clause setup that takes the carrying tx
// (may be nil when invoked from PrepareClause). The tx drives StateDB flavor
// and CREATE address derivation inside newEVM / newStateDB.
func (rt *Runtime) prepareClauseWithTx(
	clause *tx.Clause,
	clauseIndex uint32,
	gas uint64,
	txCtx *xenv.TransactionContext,
	trx *tx.Transaction,
) (exec func() (output *Output, interrupted bool, err error), interrupt func()) {
	var (
		stateDB       = rt.newStateDB(trx)
		evm           = rt.newEVM(stateDB, clauseIndex, txCtx, trx)
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
				switch e := e.(type) {
				case error:
					err = e
				case string:
					err = errors.New(e)
				default:
					err = fmt.Errorf("runtime: unknown error: %v", e)
				}
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
		if _, err := exec(); err != nil {
			return nil, err
		}
	}
	return executor.Finalize()
}

// PrepareTransaction prepare to execute tx.
func (rt *Runtime) PrepareTransaction(trx *tx.Transaction) (*TransactionExecutor, error) {
	resolvedTx, err := ResolveTransaction(trx)
	if err != nil {
		return nil, err
	}

	// ensure tx respects block boundaries
	if trx.Gas() > rt.ctx.GasLimit {
		return nil, errors.New("tx gas exceeds block gas limit")
	}

	if rt.ctx.Number >= rt.forkConfig.INTERSTELLAR && trx.Gas() > thor.MaxTxGasLimit {
		return nil, errors.New("tx gas limit exceeds the maximum allowed")
	}

	legacyTxBaseGasPrice, effectiveGasPrice, payer, _, returnGas, err := resolvedTx.BuyGas(
		rt.state,
		rt.ctx.Time,
		rt.ctx.BaseFee,
	)
	if err != nil {
		return nil, err
	}

	txCtx, err := resolvedTx.ToContext(effectiveGasPrice, payer, rt.ctx.Number, rt.chain.GetBlockID)
	if err != nil {
		return nil, err
	}

	// Spec 3 §2.1 — pre-bump origin.Nonce AFTER BuyGas (so nonce only moves
	// for txs we actually include) and BEFORE NewCheckpoint (so VMErr-driven
	// RevertTo does not undo the bump, matching eth semantics where a revert
	// still consumes the sender's sequence slot). The rt.newStateDB gate
	// makes this a no-op for V1 paths (non-ETH txs, pre-INT), so it is safe
	// to run unconditionally.
	txStateDB := rt.newStateDB(trx)
	origin := common.Address(resolvedTx.Origin)
	txStateDB.SetNonce(origin, txStateDB.GetNonce(origin)+1)

	// ResolveTransaction has checked that tx.Gas() >= IntrinsicGas
	leftOverGas := trx.Gas() - resolvedTx.IntrinsicGas
	// checkpoint to be reverted when clause failure.
	checkpoint := rt.state.NewCheckpoint()

	txOutputs := make([]*tx.Output, 0, len(resolvedTx.Clauses))
	reverted := false
	finalized := false

	hasNext := func() bool {
		return !reverted && len(txOutputs) < len(resolvedTx.Clauses)
	}

	return &TransactionExecutor{
		HasNextClause: hasNext,
		PrepareNext: func() (exec func() (bool, error), interrupt func()) {
			nextClauseIndex := uint32(len(txOutputs))
			execFunc, interrupt := rt.prepareClauseWithTx(resolvedTx.Clauses[nextClauseIndex], nextClauseIndex, leftOverGas, txCtx, trx)

			exec = func() (interrupted bool, err error) {
				if rt.vmConfig.Tracer != nil {
					rt.vmConfig.Tracer.CaptureClauseStart(leftOverGas)
					defer func() {
						rt.vmConfig.Tracer.CaptureClauseEnd(leftOverGas)
					}()
				}

				output, interrupted, err := execFunc()
				if err != nil {
					return false, err
				}

				if interrupted {
					return true, nil
				}

				gasUsed := leftOverGas - output.LeftOverGas
				leftOverGas = output.LeftOverGas

				// Apply refund counter, capped to half of the used gas.
				refund := min(gasUsed/2, output.RefundGas)

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
				txOutputs = append(txOutputs, &tx.Output{Events: output.Events, Transfers: output.Transfers})
				return
			}

			return
		},
		Finalize: func() (*tx.Receipt, error) {
			if hasNext() {
				return nil, errors.New("not all clauses processed")
			}
			if finalized {
				return nil, errors.New("already finalized")
			}
			finalized = true

			receipt := &tx.Receipt{
				Type:     trx.Type(),
				Reverted: reverted,
				Outputs:  txOutputs,
				GasUsed:  trx.Gas() - leftOverGas,
				GasPayer: payer,
			}
			receipt.Paid = new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), effectiveGasPrice)

			if err := returnGas(leftOverGas); err != nil {
				return nil, err
			}

			if !thor.IsForked(rt.ctx.Number, rt.forkConfig.GALACTICA) {
				provedWork, err := trx.ProvedWork(rt.ctx.Number-1, rt.chain.GetBlockID)
				if err != nil {
					return nil, err
				}
				overallGasPrice := trx.OverallGasPrice(legacyTxBaseGasPrice, provedWork)

				// before galactica, reward is based on the reward ratio
				rewardRatio, err := builtin.Params.Native(rt.state).Get(thor.KeyRewardRatio)
				if err != nil {
					return nil, err
				}

				reward := new(big.Int).SetUint64(receipt.GasUsed)
				reward.Mul(reward, overallGasPrice)
				reward.Mul(reward, rewardRatio)
				reward.Div(reward, big.NewInt(1e18))

				receipt.Reward = reward
			} else {
				// after galactica, reward is the priority fee
				priorityFeePerGas := trx.EffectivePriorityFeePerGas(rt.ctx.BaseFee, legacyTxBaseGasPrice, txCtx.ProvedWork)
				receipt.Reward = priorityFeePerGas.Mul(priorityFeePerGas, new(big.Int).SetUint64(receipt.GasUsed))
			}

			if err := builtin.Energy.Native(rt.state, rt.ctx.Time).Add(rt.ctx.Beneficiary, receipt.Reward); err != nil {
				return nil, err
			}
			return receipt, nil
		},
	}, nil
}
