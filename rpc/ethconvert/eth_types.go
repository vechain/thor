// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethconvert

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/tx"
)

// emptyUncleHash is the Keccak256 hash of an empty RLP list, used as sha3Uncles when
// there are no uncle blocks (always the case for VeChain).
var emptyUncleHash = common.HexToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")

// zeroNonce is an 8-byte zero block nonce — VeChain uses PoA, not PoW.
var zeroNonce = make(hexutil.Bytes, 8)

// ethBloom9 sets 3 bits in a 2048-bit (256-byte) Bloom filter for the given byte slice,
// following the Ethereum Yellow Paper Appendix H algorithm (EIP-2981).
func ethBloom9(b []byte) *big.Int {
	b = crypto.Keccak256(b)
	r := new(big.Int)
	for i := 0; i < 6; i += 2 {
		t := big.NewInt(1)
		bit := (uint(b[i+1]) + (uint(b[i]) << 8)) & 2047
		r.Or(r, t.Lsh(t, bit))
	}
	return r
}

// ethLogsBloom computes the 256-byte Ethereum bloom filter for a slice of logs.
// It ORs the bloom contribution of each log's address and topics.
func ethLogsBloom(logs []*rpc.EthLog) hexutil.Bytes {
	bin := new(big.Int)
	for _, log := range logs {
		bin.Or(bin, ethBloom9(log.Address.Bytes()))
		for _, topic := range log.Topics {
			bin.Or(bin, ethBloom9(topic[:]))
		}
	}
	bloom := make(hexutil.Bytes, 256)
	b := bin.Bytes()
	copy(bloom[256-len(b):], b)
	return bloom
}

// ToEthTx converts a TypeEthDynamicFee transaction to the Ethereum JSON representation.
// projectedIdx is the 0-based index within the ETH-only transaction subsequence of the block.
// baseFee is the block base fee used to compute effectiveGasPrice; nil is allowed (pre-GALACTICA).
func ToEthTx(t *tx.Transaction, chainID uint64, blockHash common.Hash, blockNum uint64, projectedIdx uint64, baseFee *big.Int) *rpc.EthTx {
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

	maxFee := t.MaxFeePerGas()
	gasPrice := CalcEffectiveGasPrice(maxFee, t.MaxPriorityFeePerGas(), baseFee)

	num := hexutil.Uint64(blockNum)
	idx := hexutil.Uint64(projectedIdx)
	bh := blockHash

	return &rpc.EthTx{
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

// ToEthReceipt builds an Ethereum receipt for a TypeEthDynamicFee transaction.
//
// projectedIdx    — 0-based index within the ETH-only transaction subsequence of the block.
// cumulativeGas   — cumulative gas used by ETH txs in this block up to and including this tx.
// logIndexOffset  — number of logs emitted by ETH txs before this tx in the block.
// baseFee         — block base fee; nil is allowed (pre-GALACTICA).
func ToEthReceipt(
	t *tx.Transaction,
	receipt *tx.Receipt,
	blockHash common.Hash,
	blockNum uint64,
	projectedIdx uint64,
	cumulativeGas uint64,
	logIndexOffset uint64,
	baseFee *big.Int,
) *rpc.EthReceipt {
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

	effectiveGasPrice := CalcEffectiveGasPrice(t.MaxFeePerGas(), t.MaxPriorityFeePerGas(), baseFee)

	txHash := common.Hash(t.ID())
	txIdx := hexutil.Uint64(projectedIdx)

	var logs []*rpc.EthLog
	if len(receipt.Outputs) > 0 {
		for i, event := range receipt.Outputs[0].Events {
			topics := make([]common.Hash, len(event.Topics))
			for j, tp := range event.Topics {
				topics[j] = common.Hash(tp)
			}
			logs = append(logs, &rpc.EthLog{
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
		logs = []*rpc.EthLog{}
	}

	return &rpc.EthReceipt{
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
		LogsBloom:         ethLogsBloom(logs),
		Status:            status,
		Type:              hexutil.Uint64(tx.TypeEthDynamicFee),
		EffectiveGasPrice: (*hexutil.Big)(effectiveGasPrice),
	}
}
