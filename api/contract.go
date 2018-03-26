package api

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

//ContractCallBody represents contract-call body
type ContractCallBody struct {
	Input   string              `json:"input"`
	Options ContractCallOptions `json:"options"`
}

// ContractCallOptions represents options in contract-call body
type ContractCallOptions struct {
	ClauseIndex uint32                `json:"clauseIndex"`
	Gas         uint64                `json:"gas,string"`
	From        string                `json:"from,string"`
	GasPrice    *math.HexOrDecimal256 `json:"gasPrice,string"`
	TxID        string                `json:"txID,string"`
	Value       *math.HexOrDecimal256 `json:"value,string"`
}

//ContractInterface most for call a contract
type ContractInterface struct {
	chain        *chain.Chain
	stateCreator *state.Creator
}

//NewContractInterface return a BlockMananger by chain
func NewContractInterface(chain *chain.Chain, stateCreator *state.Creator) *ContractInterface {
	return &ContractInterface{
		chain,
		stateCreator,
	}
}

func (ci *ContractInterface) defaultContractCallOptions() *ContractCallOptions {
	gp := big.NewInt(1)
	gph := math.HexOrDecimal256(*gp)
	v := big.NewInt(0)
	vh := math.HexOrDecimal256(*v)
	return &ContractCallOptions{
		ClauseIndex: 0,
		Gas:         21000,
		From:        thor.Address{}.String(),
		GasPrice:    &gph,
		TxID:        thor.Hash{}.String(),
		Value:       &vh,
	}
}

func (ci *ContractInterface) santinizeOptions(options *ContractCallOptions) {
	defaultOptions := ci.defaultContractCallOptions()

	if options.ClauseIndex < defaultOptions.ClauseIndex {
		options.ClauseIndex = defaultOptions.ClauseIndex
	}
	if options.Gas < defaultOptions.Gas {
		options.Gas = defaultOptions.Gas
	}

	if options.GasPrice == nil {
		options.GasPrice = defaultOptions.GasPrice
	} else {
		gp := big.Int(*options.GasPrice)
		gpDefault := big.Int(*defaultOptions.GasPrice)
		if (&gp).Cmp(&gpDefault) < 0 {
			options.GasPrice = defaultOptions.GasPrice
		}
	}

	if options.Value == nil {
		options.Value = defaultOptions.Value
	}

	if len(options.From) == 0 {
		options.From = defaultOptions.From
	}

	if len(options.TxID) == 0 {
		options.TxID = defaultOptions.TxID
	}
}

//Call a contract with input
func (ci *ContractInterface) Call(to *thor.Address, body *ContractCallBody) (output []byte, err error) {
	ci.santinizeOptions(&body.Options)
	blk, err := ci.chain.GetBestBlock()
	if err != nil {
		return nil, err
	}

	header := blk.Header()
	st, err := ci.stateCreator.NewState(header.StateRoot())
	if err != nil {
		return nil, err
	}
	rt := runtime.New(st, header.Beneficiary(), header.Number(), header.Timestamp(), header.GasLimit(), nil)
	v := big.Int(*body.Options.Value)
	data, err := hexutil.Decode(body.Input)
	if err != nil {
		return nil, err
	}
	clause := tx.NewClause(to).WithData(data).WithValue(&v)
	var vmout *vm.Output
	gp := big.Int(*body.Options.GasPrice)
	from, err := thor.ParseAddress(body.Options.From)
	if err != nil {
		return nil, err
	}
	txID, err := thor.ParseHash(body.Options.TxID)
	if err != nil {
		return nil, err
	}
	vmout = rt.Call(clause, body.Options.ClauseIndex, body.Options.Gas, from, &gp, txID)
	if vmout.VMErr != nil {
		return nil, vmout.VMErr
	}
	return vmout.Value, nil

}
