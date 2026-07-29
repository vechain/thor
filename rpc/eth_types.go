// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// EthBlock is the Ethereum JSON representation of a block.
// Only TypeEthDynamicFee transactions are included in the transactions field.
type EthBlock struct {
	Number     hexutil.Uint64 `json:"number"`
	Hash       common.Hash    `json:"hash"`
	ParentHash common.Hash    `json:"parentHash"`
	// Nonce is always zero — VeChain uses PoA, not PoW.
	Nonce hexutil.Bytes `json:"nonce"`
	// Sha3Uncles is the empty uncle hash — VeChain has no uncles.
	Sha3Uncles common.Hash `json:"sha3Uncles"`
	// LogsBloom is the OR of all receipt blooms for ETH-typed transactions in this block.
	LogsBloom hexutil.Bytes `json:"logsBloom"`
	// TransactionsRoot is the Keccak256 MPT root over the projected ETH transaction list.
	TransactionsRoot common.Hash `json:"transactionsRoot"`
	StateRoot        common.Hash `json:"stateRoot"`
	// ReceiptsRoot is the Keccak256 MPT root over the projected ETH receipt list.
	ReceiptsRoot common.Hash `json:"receiptsRoot"`
	// Miner is the block beneficiary declared in the VeChain block header.
	Miner           common.Address `json:"miner"`
	Difficulty      hexutil.Big    `json:"difficulty"`      // always zero (PoA)
	TotalDifficulty hexutil.Big    `json:"totalDifficulty"` // always zero (PoA)
	ExtraData       hexutil.Bytes  `json:"extraData"`
	Size            hexutil.Uint64 `json:"size"`
	GasLimit        hexutil.Uint64 `json:"gasLimit"`
	// GasUsed is the sum of gas used by TypeEthDynamicFee transactions only.
	GasUsed   hexutil.Uint64 `json:"gasUsed"`
	Timestamp hexutil.Uint64 `json:"timestamp"`
	// BaseFeePerGas is omitted for pre-GALACTICA blocks (nil BaseFee on header).
	BaseFeePerGas *hexutil.Big `json:"baseFeePerGas,omitempty"`
	// Transactions is either []common.Hash (fullTx=false) or []*EthTx (fullTx=true).
	Transactions any           `json:"transactions"`
	Uncles       []common.Hash `json:"uncles"`
}

// EthTx is the Ethereum JSON representation of a TypeEthDynamicFee transaction.
type EthTx struct {
	BlockHash            *common.Hash    `json:"blockHash"`
	BlockNumber          *hexutil.Uint64 `json:"blockNumber"`
	From                 common.Address  `json:"from"`
	Gas                  hexutil.Uint64  `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Hash                 common.Hash     `json:"hash"`
	Input                hexutil.Bytes   `json:"input"`
	Nonce                hexutil.Uint64  `json:"nonce"`
	To                   *common.Address `json:"to"`
	TransactionIndex     *hexutil.Uint64 `json:"transactionIndex"`
	Value                *hexutil.Big    `json:"value"`
	Type                 hexutil.Uint64  `json:"type"`
	ChainID              *hexutil.Big    `json:"chainId"`
	V                    *hexutil.Big    `json:"v"`
	R                    *hexutil.Big    `json:"r"`
	S                    *hexutil.Big    `json:"s"`
}

// EthLog is the Ethereum JSON representation of a contract event log.
type EthLog struct {
	Address     common.Address `json:"address"`
	Topics      []common.Hash  `json:"topics"`
	Data        hexutil.Bytes  `json:"data"`
	BlockNumber hexutil.Uint64 `json:"blockNumber"`
	TxHash      common.Hash    `json:"transactionHash"`
	TxIndex     hexutil.Uint64 `json:"transactionIndex"`
	BlockHash   common.Hash    `json:"blockHash"`
	LogIndex    hexutil.Uint64 `json:"logIndex"`
	Removed     bool           `json:"removed"`
}

// EthReceipt is the Ethereum JSON representation of a TypeEthDynamicFee transaction receipt.
type EthReceipt struct {
	TransactionHash   common.Hash     `json:"transactionHash"`
	TransactionIndex  hexutil.Uint64  `json:"transactionIndex"`
	BlockHash         common.Hash     `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	From              common.Address  `json:"from"`
	To                *common.Address `json:"to"`
	GasUsed           hexutil.Uint64  `json:"gasUsed"`
	CumulativeGasUsed hexutil.Uint64  `json:"cumulativeGasUsed"`
	ContractAddress   *common.Address `json:"contractAddress"`
	Logs              []*EthLog       `json:"logs"`
	// LogsBloom is computed from the ETH-typed transaction's event logs (bloom9 over address and topics).
	LogsBloom hexutil.Bytes `json:"logsBloom"`
	// Status: 1 = success, 0 = reverted.
	Status hexutil.Uint64 `json:"status"`
	// Type is always 2 (EIP-1559).
	Type              hexutil.Uint64 `json:"type"`
	EffectiveGasPrice *hexutil.Big   `json:"effectiveGasPrice"`
}
