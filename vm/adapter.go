package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
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

// vmContractRef implements evm.ContractRef
type vmContractRef struct {
	addr acc.Address
}

func (vc *vmContractRef) Address() common.Address {
	return common.Address(vc.addr)
}
