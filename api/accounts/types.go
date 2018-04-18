package accounts

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

//Account for marshal account
type Account struct {
	Balance math.HexOrDecimal256 `json:"balance,string"`
	Energy  math.HexOrDecimal256 `json:"energy,string"`
	HasCode bool                 `json:"hasCode"`
}

//ContractCall represents contract-call body
type ContractCall struct {
	Value    *math.HexOrDecimal256 `json:"value,string"`
	Data     string                `json:"data"`
	Gas      uint64                `json:"gas"`
	GasPrice *math.HexOrDecimal256 `json:"gasPrice,string"`
	Caller   thor.Address          `json:"caller"`
}

type VMOutput struct {
	Data     string `json:"data"`
	GasUsed  uint64 `json:"gasUsed"`
	Reverted bool   `json:"reverted"`
	VMError  string `json:"vmError"`
}

func convertVMOutputWithInputGas(vo *vm.Output, inputGas uint64) *VMOutput {
	gasUsed := inputGas - vo.LeftOverGas
	var (
		vmError  string
		reverted bool
	)

	if vo.VMErr != nil {
		reverted = true
		vmError = vo.VMErr.Error()
	}

	return &VMOutput{
		Data:     hexutil.Encode(vo.Value),
		GasUsed:  gasUsed,
		Reverted: reverted,
		VMError:  vmError,
	}
}
