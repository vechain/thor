// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethcompat

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// rpcBlock is the Ethereum JSON wire representation of a block.
type rpcBlock struct {
	Number           hexutil.Uint64  `json:"number"`
	Hash             common.Hash     `json:"hash"`
	ParentHash       common.Hash     `json:"parentHash"`
	Nonce            hexutil.Bytes   `json:"nonce"`
	SHA3Uncles       common.Hash     `json:"sha3Uncles"`
	LogsBloom        hexutil.Bytes   `json:"logsBloom"`
	TransactionsRoot common.Hash     `json:"transactionsRoot"`
	StateRoot        common.Hash     `json:"stateRoot"`
	ReceiptsRoot     common.Hash     `json:"receiptsRoot"`
	Miner            common.Address  `json:"miner"`
	Difficulty       hexutil.Uint64  `json:"difficulty"`
	TotalDifficulty  hexutil.Uint64  `json:"totalDifficulty"`
	ExtraData        hexutil.Bytes   `json:"extraData"`
	Size             hexutil.Uint64  `json:"size"`
	GasLimit         hexutil.Uint64  `json:"gasLimit"`
	GasUsed          hexutil.Uint64  `json:"gasUsed"`
	Timestamp        hexutil.Uint64  `json:"timestamp"`
	Transactions     any             `json:"transactions"`
	Uncles           []common.Hash   `json:"uncles"`
	BaseFeePerGas    *hexutil.Big    `json:"baseFeePerGas,omitempty"`
}

// rpcTransaction is the Ethereum JSON wire representation of a transaction.
type rpcTransaction struct {
	BlockHash            *common.Hash    `json:"blockHash"`
	BlockNumber          *hexutil.Uint64 `json:"blockNumber"`
	From                 common.Address  `json:"from"`
	Gas                  hexutil.Uint64  `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas,omitempty"`
	Hash                 common.Hash     `json:"hash"`
	Input                hexutil.Bytes   `json:"input"`
	Nonce                hexutil.Uint64  `json:"nonce"`
	To                   *common.Address `json:"to"`
	TransactionIndex     *hexutil.Uint64 `json:"transactionIndex"`
	Value                *hexutil.Big    `json:"value"`
	Type                 hexutil.Uint64  `json:"type"`
	ChainID              *hexutil.Big    `json:"chainId,omitempty"`
	V                    *hexutil.Big    `json:"v"`
	R                    *hexutil.Big    `json:"r"`
	S                    *hexutil.Big    `json:"s"`
}

// rpcReceipt is the Ethereum JSON wire representation of a transaction receipt.
type rpcReceipt struct {
	TransactionHash   common.Hash     `json:"transactionHash"`
	TransactionIndex  hexutil.Uint64  `json:"transactionIndex"`
	BlockHash         common.Hash     `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	From              common.Address  `json:"from"`
	To                *common.Address `json:"to"`
	CumulativeGasUsed hexutil.Uint64  `json:"cumulativeGasUsed"`
	GasUsed           hexutil.Uint64  `json:"gasUsed"`
	ContractAddress   *common.Address `json:"contractAddress"`
	Logs              []*rpcLog       `json:"logs"`
	LogsBloom         hexutil.Bytes   `json:"logsBloom"`
	Type              hexutil.Uint64  `json:"type"`
	Status            hexutil.Uint64  `json:"status"`
	EffectiveGasPrice *hexutil.Big    `json:"effectiveGasPrice"`
}

// rpcLog is the Ethereum JSON wire representation of a log entry.
type rpcLog struct {
	Address          common.Address `json:"address"`
	Topics           []common.Hash  `json:"topics"`
	Data             hexutil.Bytes  `json:"data"`
	BlockNumber      hexutil.Uint64 `json:"blockNumber"`
	TransactionHash  common.Hash    `json:"transactionHash"`
	TransactionIndex hexutil.Uint64 `json:"transactionIndex"`
	BlockHash        common.Hash    `json:"blockHash"`
	LogIndex         hexutil.Uint64 `json:"logIndex"`
	Removed          bool           `json:"removed"`
}

// callArgs represents the call object used in eth_call and eth_estimateGas.
type callArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Data                 *hexutil.Bytes  `json:"data"`
	Input                *hexutil.Bytes  `json:"input"`
}

// logFilter represents the filter object used in eth_getLogs.
type logFilter struct {
	FromBlock *string      `json:"fromBlock"`
	ToBlock   *string      `json:"toBlock"`
	BlockHash *common.Hash `json:"blockHash"`
	Address   any          `json:"address"` // string | []string | null
	Topics    []any        `json:"topics"`  // array of (string | []string | null)
}

// zeroBloom is 256 zero bytes used as a placeholder for logs bloom.
var zeroBloom = make(hexutil.Bytes, 256)

// zeroNonce is 8 zero bytes used as the block nonce placeholder.
var zeroNonce = make(hexutil.Bytes, 8)
