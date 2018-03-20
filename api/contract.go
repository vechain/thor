package api

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"math/big"
)

//ContractInterfaceOptions contract calling options
type ContractInterfaceOptions struct {
	Index    uint32                `json:"index"`
	Gas      uint64                `json:"gas,string"`
	From     string                `json:"from,string"`
	GasPrice *math.HexOrDecimal256 `json:"gasPrice,string"`
	TxID     string                `json:"txID,string"`
	Value    *math.HexOrDecimal256 `json:"value,string"`
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
	gp := big.NewInt(40)
	gph := math.HexOrDecimal256(*gp)
	v := big.NewInt(0)
	vh := math.HexOrDecimal256(*v)
	return &ContractInterfaceOptions{
		Index:    1,
		Gas:      10000,
		From:     thor.Address{}.String(),
		GasPrice: &gph,
		TxID:     thor.Hash{}.String(),
		Value:    &vh,
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
	gp := big.Int(*options.GasPrice)
	gp1 := big.Int(*ops.GasPrice)
	if (&gp).Cmp(&gp1) < 0 {
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
	v := big.Int(*options.Value)
	clause := tx.NewClause(to).WithData(input).WithValue(&v)
	var vmout *vm.Output
	gp := big.Int(*options.GasPrice)
	from, err := thor.ParseAddress(options.From)
	if err != nil {
		return nil, err
	}
	txID, err := thor.ParseHash(options.TxID)
	if err != nil {
		return nil, err
	}
	vmout = rt.Call(clause, options.Index, options.Gas, from, &gp, txID)
	if vmout.VMErr != nil {
		return nil, vmout.VMErr
	}
	return vmout.Value, nil

}
