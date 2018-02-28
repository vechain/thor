package api

import (
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"math/big"
)

//ContractInterfaceOptions contract calling options
type ContractInterfaceOptions struct {
	Index    uint32
	Gas      uint64
	From     thor.Address
	GasPrice *big.Int
	TxID     thor.Hash
	Value    *big.Int
}

//ContractInterface most for call a contract
type ContractInterface struct {
	bestBlkGetter bestBlockGetter
	stateCreator  *state.Creator
}

//NewContractInterface return a BlockMananger by chain
func NewContractInterface(bestBlkGetter bestBlockGetter, stateCreator *state.Creator) *ContractInterface {
	return &ContractInterface{
		bestBlkGetter: bestBlkGetter,
		stateCreator:  stateCreator,
	}
}

//DefaultContractInterfaceOptions a default contract options
func (ci *ContractInterface) DefaultContractInterfaceOptions() *ContractInterfaceOptions {
	return &ContractInterfaceOptions{
		Index:    1,
		Gas:      100000,
		From:     thor.Address{},
		GasPrice: big.NewInt(40),
		TxID:     thor.Hash{},
		Value:    big.NewInt(0),
	}
}

func (ci *ContractInterface) santinizeOptions(options *ContractInterfaceOptions) {
	ops := ci.DefaultContractInterfaceOptions()
	if options.Index < ops.Index {
		options.Index = ops.Index
	}
	if options.Gas < ops.Gas {
		options.Gas = ops.Gas
	}
	if options.GasPrice.Cmp(ops.GasPrice) < 0 {
		options.GasPrice = ops.GasPrice
	}
	if options.Value == nil {
		options.Value = ops.Value
	}
}

//Call a contract with input
func (ci *ContractInterface) Call(to *thor.Address, input []byte, options *ContractInterfaceOptions) (output []byte, err error) {
	ci.santinizeOptions(options)
	blk, err := ci.bestBlkGetter.GetBestBlock()
	if err != nil {
		return nil, err
	}

	header := blk.Header()
	st, err := ci.stateCreator.NewState(header.StateRoot())
	if err != nil {
		return nil, err
	}
	rt := runtime.New(st, header.Beneficiary(), header.Number(), header.Timestamp(), header.GasLimit(), nil)
	clause := tx.NewClause(to).WithData(input).WithValue(options.Value)
	var vmout *vm.Output
	vmout = rt.Call(clause, options.Index, options.Gas, options.From, options.GasPrice, options.TxID)
	if vmout.VMErr != nil {
		return nil, vmout.VMErr
	}
	return vmout.Value, nil

}
