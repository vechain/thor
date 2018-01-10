package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/thor"
)

// vmContractRef implements evm.ContractRef
type vmContractRef struct {
	addr thor.Address
}

func (vc *vmContractRef) Address() common.Address {
	return common.Address(vc.addr)
}
