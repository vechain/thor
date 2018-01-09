package runtime

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

type Runtime struct {
	vmConfig vm.Config
	getHash  func(uint64) cry.Hash
	state    State

	// block env
	blockBeneficiary acc.Address
	blockNumber      uint32
	blockTime        uint64
	blockGasLimit    uint64
}

func New(
	state State,
	header *block.Header,
	getHash func(uint64) cry.Hash,
	vmConfig vm.Config,
) *Runtime {
	return &Runtime{
		vmConfig,
		getHash,
		state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit()}
}

func (rt *Runtime) Exec(
	clause *tx.Clause,
	index int,
	gas uint64,
	origin acc.Address,
	gasPrice *big.Int,
	txHash cry.Hash,
) *vm.Output {

	ctx := vm.Context{
		Beneficiary: rt.blockBeneficiary,
		BlockNumber: new(big.Int).SetUint64(uint64(rt.blockNumber)),
		Time:        new(big.Int).SetUint64(rt.blockTime),
		GasLimit:    new(big.Int).SetUint64(rt.blockGasLimit),

		Origin:   origin,
		GasPrice: gasPrice,
		TxHash:   txHash,

		GetHash:     rt.getHash,
		ClauseIndex: uint64(index),
	}

	if clause.To == nil {
		return vm.New(ctx, rt.state, rt.vmConfig).
			Create(origin, clause.Data, gas, clause.Value.ToBig())
	}
	return vm.New(ctx, rt.state, rt.vmConfig).
		Call(origin, *clause.To, clause.Data, gas, clause.Value.ToBig())
}

// func (rt *Runtime) consumeEnergy(who acc.Address, amount *big.Int) (*big.Int, *big.Int, error) {
// 	return nil, nil, nil
// }

// func (rt *Runtime) chargeEnergy(who acc.Address, amount *big.Int) error {
// 	return nil
// }

// func (rt *Runtime) ExecTransaction(tx *tx.Transaction) (*tx.Receipt, error) {
// 	origin, err := tx.Signer()
// 	if err != nil {
// 		return nil, err
// 	}

// 	bigIntrinsicGas := tx.IntrinsicGas()
// 	if bigIntrinsicGas.BitLen() > 64 {
// 		return nil, evm.ErrOutOfGas
// 	}
// 	intrinsicGas := bigIntrinsicGas.Uint64()

// 	bigGasLimit := tx.GasLimit().ToBig()
// 	if bigGasLimit.BitLen() > 64 {
// 		return nil, evm.ErrOutOfGas
// 	}
// 	gasLimit := bigGasLimit.Uint64()

// 	if intrinsicGas > gasLimit {
// 		return nil, evm.ErrOutOfGas
// 	}

// 	gasPrice := tx.GasPrice().ToBig()

// 	selfConsumed, shareConsumed, err := rt.consumeEnergy(*origin, new(big.Int).Mul(gasPrice, bigGasLimit))
// 	if err != nil {
// 		return nil, err
// 	}

// 	leftOverGas := gasLimit - intrinsicGas

// 	totalRefund := new(big.Int)
// 	txHash := tx.Hash()

// 	checkpoint := rt.state.NewCheckpoint()

// 	for i, clause := range tx.Clauses() {
// 		output := rt.Exec(clause, i, leftOverGas, *origin, gasPrice, txHash)
// 		if output.VMErr != nil {

// 		}
// 		if err != nil
// 		if vmErr := output.vmOutput; vmErr != nil {
// 			break
// 		}
// 		gasUsed := leftOverGas - output.LeftOverGas
// 		leftOverGas = output.LeftOverGas

// 		// Apply refund counter, capped to half of the used gas.
// 		halfUsed := new(big.Int).SetUint64(gasUsed / 2)
// 		refund := math.BigMin(output.RefundGas, halfUsed)
// 		totalRefund.Add(totalRefund, refund)

// 		leftOverGas += refund.Uint64()
// 	}

// 	if vmErr != nil {

// 	}

// 	return nil, nil
// }
