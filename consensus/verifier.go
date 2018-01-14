package consensus

import (
	"math"
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func verify(blk *block.Block, preHeader *block.Header, state *state.State, sign *cry.Signing) error {
	header := blk.Header()
	signer, err := sign.Signer(header)
	if err != nil {
		return err
	}

	rt := runtime.New(state, preHeader, nil)
	proposers := Authority(rt, "getProposers")
	legal, absentee, err := schedule.New(
		proposers,
		Authority(rt, "getAbsentee"),
		preHeader.Number(),
		preHeader.Timestamp()).Validate(signer, header.Timestamp())

	if !legal {
		return errSinger
	}
	if err != nil {
		return err
	}

	if preHeader.TotalScore()+uint64(len(proposers)-len(absentee)) != header.TotalScore() {
		return errTotalScore
	}

	receiptsRoot, gasUsed, energyUsed, err := ProcessBlock(state, blk, sign)
	if err != nil {
		return err
	}

	if header.ReceiptsRoot() != receiptsRoot {
		return errReceiptsRoot
	}

	if header.GasUsed() != gasUsed {
		return errGasUsed
	}

	data := contracts.Energy.PackCharge(header.Beneficiary(), new(big.Int).SetUint64(energyUsed))

	output := rt.Execute(&tx.Clause{
		To:   &contracts.Energy.Address,
		Data: data,
	}, 0, math.MaxUint64, contracts.Energy.Address, &big.Int{}, thor.Hash{})
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "charge energy")
	}

	data, err = contracts.Energy.ABI.Pack("absent", absentee)
	if err != nil {
		panic(err)
	}
	output = rt.Execute(&tx.Clause{
		To:   &contracts.Authority.Address,
		Data: data,
	}, 0, math.MaxUint64, contracts.Authority.Address, &big.Int{}, thor.Hash{})
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "set absent")
	}

	if stateRoot, err := state.Stage().Hash(); err == nil {
		if header.StateRoot() != stateRoot {
			return errStateRoot
		}
	} else {
		return err
	}

	return nil
}

func getHash(uint64) thor.Hash {
	return thor.Hash{}
}

// ProcessBlock can execute all transactions in a block.
func ProcessBlock(state *state.State, blk *block.Block, sign *cry.Signing) (thor.Hash, uint64, uint64, error) {
	rt := runtime.New(state, blk.Header(), getHash)
	receipts, totalGasUsed, totalEnergyUsed, err := processTransactions(rt, blk.Transactions(), sign)
	if err != nil {
		return thor.Hash{}, 0, 0, err
	}
	return receipts.RootHash(), totalGasUsed, totalEnergyUsed, nil
}

func processTransactions(rt *runtime.Runtime, transactions tx.Transactions, sign *cry.Signing) (tx.Receipts, uint64, uint64, error) {
	length := len(transactions)
	if length == 0 {
		return nil, 0, 0, nil
	}

	receipt, _, err := rt.ExecuteTransaction(transactions[0], sign)
	if err != nil {
		return nil, 0, 0, err
	}
	energyUsed := receipt.GasUsed * transactions[0].GasPrice().Uint64()

	receipts, totalGasUsed, totalEnergyUsed, err := processTransactions(rt, transactions[1:length], sign)
	if err != nil {
		return nil, 0, 0, err
	}

	return append(receipts, receipt), totalGasUsed + receipt.GasUsed, totalEnergyUsed + energyUsed, nil
}
