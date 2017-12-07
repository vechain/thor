package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/vecore/cry"
)

// vmContractRef is a adapter from ContractRef to vm.ContractRef
type vmContractRef struct {
	ContractRef
}

func (w *vmContractRef) Address() common.Address {
	return common.Address(w.ContractRef.Address())
}

// vmChainContext is a adapter from ChainContext to core.ChainContext
type vmChainContext struct {
	ChainContext
}

func (vc *vmChainContext) GetHeader(hash common.Hash, num uint64) *types.Header {
	return vc.ChainContext.GetHeader(cry.Hash(hash), num)
}

// vmMessage is a adapter from Message to core.Message
type vmMessage struct {
	Message
}

func (vmsg *vmMessage) From() common.Address {
	return common.Address(vmsg.Message.From())
}

func (vmsg *vmMessage) To() *common.Address {
	return (*common.Address)(vmsg.Message.To())
}
