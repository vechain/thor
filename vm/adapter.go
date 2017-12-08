package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
)

func translation2EthHeader(header *block.Header) *types.Header {
	ethHeader := new(types.Header)
	ethHeader.ParentHash = common.Hash(header.ParentHash())
	ethHeader.Coinbase = common.Address(header.Beneficiary())
	ethHeader.Root = common.Hash(header.StateRoot())
	ethHeader.TxHash = common.Hash(header.TxsRoot())
	ethHeader.ReceiptHash = common.Hash(header.ReceiptsRoot())
	ethHeader.GasLimit = header.GasLimit()
	ethHeader.GasUsed = header.GasUsed()
	ethHeader.Time = new(big.Int).SetUint64(header.Timestamp())
	ethHeader.Number = new(big.Int).SetUint64(uint64(header.Number()))
	return ethHeader
}

// vmContractRef is a adapter from ContractRef to evm.ContractRef
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
	return translation2EthHeader(vc.ChainContext.GetHeader(cry.Hash(hash), num))
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
