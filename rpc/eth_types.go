// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/tx"
)

// EthBlock is the Ethereum JSON representation of a block.
// Only TypeEthTyped1559 transactions are included in the transactions field.
type EthBlock struct {
	Number     hexutil.Uint64 `json:"number"`
	Hash       common.Hash    `json:"hash"`
	ParentHash common.Hash    `json:"parentHash"`
	// Nonce is always zero — VeChain uses PoA, not PoW.
	Nonce hexutil.Bytes `json:"nonce"`
	// Sha3Uncles is the empty uncle hash — VeChain has no uncles.
	Sha3Uncles common.Hash `json:"sha3Uncles"`
	// TODO: compute from ETH tx logs once Phase 1 is complete.
	LogsBloom hexutil.Bytes `json:"logsBloom"`
	// TODO: compute Merkle root over projected ETH transactions.
	TransactionsRoot common.Hash `json:"transactionsRoot"`
	StateRoot        common.Hash `json:"stateRoot"`
	// TODO: compute Merkle root over projected ETH receipts.
	ReceiptsRoot common.Hash `json:"receiptsRoot"`
	// Miner is the block beneficiary declared in the VeChain block header.
	Miner           common.Address `json:"miner"`
	Difficulty      hexutil.Big    `json:"difficulty"`      // always zero (PoA)
	TotalDifficulty hexutil.Big    `json:"totalDifficulty"` // always zero (PoA)
	ExtraData       hexutil.Bytes  `json:"extraData"`
	Size            hexutil.Uint64 `json:"size"`
	GasLimit        hexutil.Uint64 `json:"gasLimit"`
	// GasUsed is the sum of gas used by TypeEthTyped1559 transactions only.
	GasUsed   hexutil.Uint64 `json:"gasUsed"`
	Timestamp hexutil.Uint64 `json:"timestamp"`
	// BaseFeePerGas is omitted for pre-GALACTICA blocks (nil BaseFee on header).
	BaseFeePerGas *hexutil.Big `json:"baseFeePerGas,omitempty"`
	// Transactions is either []common.Hash (fullTx=false) or []*EthTx (fullTx=true).
	Transactions any           `json:"transactions"`
	Uncles       []common.Hash `json:"uncles"`
}

// emptyUncleHash is the Keccak256 hash of an empty RLP list, used as sha3Uncles when
// there are no uncle blocks (always the case for VeChain).
var emptyUncleHash = common.HexToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")

// zeroLogsBloom is a 256-byte zero bloom filter returned as a placeholder.
// TODO: compute from ETH tx events once Phase 1 bloom computation is implemented.
var zeroLogsBloom = make(hexutil.Bytes, 256)

// zeroNonce is an 8-byte zero block nonce — VeChain uses PoA, not PoW.
var zeroNonce = make(hexutil.Bytes, 8)

// EthTx is the Ethereum JSON representation of a TypeEthTyped1559 transaction.
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

// ToEthTx converts a TypeEthTyped1559 transaction to the Ethereum JSON representation.
// projectedIdx is the 0-based index within the ETH-only transaction subsequence of the block.
// baseFee is the block base fee used to compute effectiveGasPrice; nil is allowed (pre-GALACTICA).
func ToEthTx(t *tx.Transaction, chainID uint64, blockHash common.Hash, blockNum uint64, projectedIdx uint64, baseFee *big.Int) *EthTx {
	origin, _ := t.Origin()
	clauses := t.Clauses()

	var to *common.Address
	if clauses[0].To() != nil {
		addr := common.Address(*clauses[0].To())
		to = &addr
	}

	// EIP-1559 signature layout: [R(32) || S(32) || yParity(1)]
	sig := t.Signature()
	r := new(big.Int).SetBytes(sig[0:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetUint64(uint64(sig[64])) // yParity: 0 or 1

	// effectiveGasPrice = min(maxFeePerGas, baseFee + maxPriorityFeePerGas)
	// Fall back to maxFeePerGas when baseFee is unavailable (pre-GALACTICA blocks).
	maxFee := t.MaxFeePerGas()
	gasPrice := new(big.Int).Set(maxFee)
	if baseFee != nil {
		effective := new(big.Int).Add(baseFee, t.MaxPriorityFeePerGas())
		if effective.Cmp(gasPrice) < 0 {
			gasPrice = effective
		}
	}

	num := hexutil.Uint64(blockNum)
	idx := hexutil.Uint64(projectedIdx)
	bh := blockHash

	return &EthTx{
		BlockHash:            &bh,
		BlockNumber:          &num,
		From:                 common.Address(origin),
		Gas:                  hexutil.Uint64(t.Gas()),
		GasPrice:             (*hexutil.Big)(gasPrice),
		MaxFeePerGas:         (*hexutil.Big)(maxFee),
		MaxPriorityFeePerGas: (*hexutil.Big)(t.MaxPriorityFeePerGas()),
		Hash:                 common.Hash(t.ID()),
		Input:                clauses[0].Data(),
		Nonce:                hexutil.Uint64(t.Nonce()),
		To:                   to,
		TransactionIndex:     &idx,
		Value:                (*hexutil.Big)(new(big.Int).Set(clauses[0].Value())),
		Type:                 hexutil.Uint64(tx.TypeEthDynamicFee),
		ChainID:              (*hexutil.Big)(new(big.Int).SetUint64(chainID)),
		V:                    (*hexutil.Big)(v),
		R:                    (*hexutil.Big)(r),
		S:                    (*hexutil.Big)(s),
	}
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

// EthReceipt is the Ethereum JSON representation of a TypeEthTyped1559 transaction receipt.
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
	// TODO: compute bloom filter from ETH tx event logs.
	LogsBloom hexutil.Bytes `json:"logsBloom"`
	// Status: 1 = success, 0 = reverted.
	Status hexutil.Uint64 `json:"status"`
	// Type is always 2 (EIP-1559).
	Type              hexutil.Uint64 `json:"type"`
	EffectiveGasPrice *hexutil.Big   `json:"effectiveGasPrice"`
}

// ToEthReceipt builds an Ethereum receipt for a TypeEthTyped1559 transaction.
//
// projectedIdx    — 0-based index within the ETH-only transaction subsequence of the block.
// cumulativeGas   — cumulative gas used by ETH txs in this block up to and including this tx.
// logIndexOffset  — number of logs emitted by ETH txs before this tx in the block.
// baseFee         — block base fee; nil is allowed (pre-GALACTICA).
func ToEthReceipt(
	t *tx.Transaction,
	receipt *tx.Receipt,
	chainID uint64,
	blockHash common.Hash,
	blockNum uint64,
	projectedIdx uint64,
	cumulativeGas uint64,
	logIndexOffset uint64,
	baseFee *big.Int,
) *EthReceipt {
	origin, _ := t.Origin()
	clauses := t.Clauses()

	var to *common.Address
	if clauses[0].To() != nil {
		addr := common.Address(*clauses[0].To())
		to = &addr
	}

	// contractAddress is re-derived for CREATE transactions (To == nil).
	// EIP-1559 CREATE always uses crypto.CreateAddress(sender, nonce).
	var contractAddress *common.Address
	if to == nil {
		addr := crypto.CreateAddress(common.Address(origin), t.Nonce())
		contractAddress = &addr
	}

	status := hexutil.Uint64(1)
	if receipt.Reverted {
		status = 0
	}

	maxFee := t.MaxFeePerGas()
	effectiveGasPrice := new(big.Int).Set(maxFee)
	if baseFee != nil {
		effective := new(big.Int).Add(baseFee, t.MaxPriorityFeePerGas())
		if effective.Cmp(effectiveGasPrice) < 0 {
			effectiveGasPrice = effective
		}
	}

	txHash := common.Hash(t.ID())
	txIdx := hexutil.Uint64(projectedIdx)

	var logs []*EthLog
	if len(receipt.Outputs) > 0 {
		for i, event := range receipt.Outputs[0].Events {
			topics := make([]common.Hash, len(event.Topics))
			for j, tp := range event.Topics {
				topics[j] = common.Hash(tp)
			}
			logs = append(logs, &EthLog{
				Address:     common.Address(event.Address),
				Topics:      topics,
				Data:        event.Data,
				BlockNumber: hexutil.Uint64(blockNum),
				TxHash:      txHash,
				TxIndex:     txIdx,
				BlockHash:   blockHash,
				LogIndex:    hexutil.Uint64(logIndexOffset + uint64(i)),
				Removed:     false,
			})
		}
	}
	if logs == nil {
		logs = []*EthLog{}
	}

	return &EthReceipt{
		TransactionHash:   txHash,
		TransactionIndex:  txIdx,
		BlockHash:         blockHash,
		BlockNumber:       hexutil.Uint64(blockNum),
		From:              common.Address(origin),
		To:                to,
		GasUsed:           hexutil.Uint64(receipt.GasUsed),
		CumulativeGasUsed: hexutil.Uint64(cumulativeGas),
		ContractAddress:   contractAddress,
		Logs:              logs,
		LogsBloom:         zeroLogsBloom,
		Status:            status,
		Type:              hexutil.Uint64(tx.TypeEthDynamicFee),
		EffectiveGasPrice: (*hexutil.Big)(effectiveGasPrice),
	}
}
