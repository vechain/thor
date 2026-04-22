// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethcompat

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// thorHashToEth converts a VeChain Bytes32 to an Ethereum common.Hash.
func thorHashToEth(b thor.Bytes32) common.Hash {
	return common.Hash(b)
}

// thorAddrToEth converts a VeChain Address to an Ethereum common.Address.
func thorAddrToEth(a thor.Address) common.Address {
	return common.Address(a)
}

// ethAddrToThor converts an Ethereum common.Address to a VeChain Address.
func ethAddrToThor(a common.Address) thor.Address {
	return thor.Address(a)
}

// convertBlock converts a VeChain block to Ethereum JSON wire format.
// When fullTxs is true the Transactions field holds []*rpcTransaction,
// otherwise it holds []common.Hash. Only EIP-1559 transactions are included.
func convertBlock(blk *block.Block, summary *chain.BlockSummary, chainID uint64, fullTxs bool) *rpcBlock {
	h := blk.Header()
	txs := blk.Transactions()

	var transactions any
	if fullTxs {
		list := make([]*rpcTransaction, 0)
		for i, t := range txs {
			if t.Type() != tx.TypeEthTyped1559 {
				continue
			}
			list = append(list, convertTx(t, h, uint64(i), chainID))
		}
		transactions = list
	} else {
		hashes := make([]common.Hash, 0)
		for _, t := range txs {
			if t.Type() != tx.TypeEthTyped1559 {
				continue
			}
			hashes = append(hashes, thorHashToEth(t.ID()))
		}
		transactions = hashes
	}

	// Always include baseFeePerGas so hardhat/foundry/cast treat this chain as EIP-1559 ready
	// and send type-0x02 transactions. For blocks produced before the INTERSTELLAR fork
	// activates (e.g. genesis block 0 in solo mode) the header has no base fee; fall back to
	// InitialBaseFee which is what block 1 will carry.
	baseFee := h.BaseFee()
	if baseFee == nil {
		baseFee = new(big.Int).SetUint64(thor.InitialBaseFee)
	}

	return &rpcBlock{
		Number:           hexutil.Uint64(h.Number()),
		Hash:             thorHashToEth(h.ID()),
		ParentHash:       thorHashToEth(h.ParentID()),
		Nonce:            zeroNonce,
		SHA3Uncles:       common.Hash{},
		LogsBloom:        zeroBloom,
		TransactionsRoot: thorHashToEth(h.TxsRoot()),
		StateRoot:        thorHashToEth(h.StateRoot()),
		ReceiptsRoot:     thorHashToEth(h.ReceiptsRoot()),
		Miner:            thorAddrToEth(h.Beneficiary()),
		Difficulty:       0,
		TotalDifficulty:  0,
		ExtraData:        hexutil.Bytes{},
		Size:             hexutil.Uint64(summary.Size),
		GasLimit:         hexutil.Uint64(h.GasLimit()),
		GasUsed:          hexutil.Uint64(h.GasUsed()),
		Timestamp:        hexutil.Uint64(h.Timestamp()),
		Transactions:     transactions,
		Uncles:           []common.Hash{},
		BaseFeePerGas:    (*hexutil.Big)(baseFee),
	}
}

// convertTx converts a VeChain EIP-1559 transaction to Ethereum JSON wire format.
// txIndex is the position of the transaction within the block (used for TransactionIndex).
func convertTx(t *tx.Transaction, header *block.Header, txIndex uint64, chainID uint64) *rpcTransaction {
	origin, _ := t.Origin()
	clauses := t.Clauses()

	var toAddr *common.Address
	var inputData hexutil.Bytes
	if len(clauses) > 0 {
		if clauses[0].To() != nil {
			a := thorAddrToEth(*clauses[0].To())
			toAddr = &a
		}
		inputData = clauses[0].Data()
	}

	var value *big.Int
	if len(clauses) > 0 && clauses[0].Value() != nil {
		value = new(big.Int).Set(clauses[0].Value())
	} else {
		value = new(big.Int)
	}

	sig := t.Signature() // 65 bytes: r[0:32] | s[32:64] | yParity[64]
	r := new(big.Int).SetBytes(sig[0:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetUint64(uint64(sig[64]))

	gasPrice := t.MaxFeePerGas()
	if gasPrice == nil {
		gasPrice = new(big.Int)
	}

	rt := &rpcTransaction{
		From:                 thorAddrToEth(origin),
		Gas:                  hexutil.Uint64(t.Gas()),
		GasPrice:             (*hexutil.Big)(gasPrice),
		MaxFeePerGas:         (*hexutil.Big)(t.MaxFeePerGas()),
		MaxPriorityFeePerGas: (*hexutil.Big)(t.MaxPriorityFeePerGas()),
		Hash:                 thorHashToEth(t.ID()),
		Input:                inputData,
		Nonce:                hexutil.Uint64(t.Nonce()),
		To:                   toAddr,
		Value:                (*hexutil.Big)(value),
		Type:                 hexutil.Uint64(tx.TypeEthTyped1559),
		ChainID:              (*hexutil.Big)(new(big.Int).SetUint64(chainID)),
		V:                    (*hexutil.Big)(v),
		R:                    (*hexutil.Big)(r),
		S:                    (*hexutil.Big)(s),
	}

	if header != nil {
		bh := thorHashToEth(header.ID())
		bn := hexutil.Uint64(header.Number())
		ti := hexutil.Uint64(txIndex)
		rt.BlockHash = &bh
		rt.BlockNumber = &bn
		rt.TransactionIndex = &ti
	}
	return rt
}

// convertReceipt converts a VeChain receipt to Ethereum JSON wire format.
func convertReceipt(receipt *tx.Receipt, t *tx.Transaction, header *block.Header, txIndex uint64, chainID uint64) *rpcReceipt {
	origin, _ := t.Origin()
	clauses := t.Clauses()

	var toAddr *common.Address
	if len(clauses) > 0 && clauses[0].To() != nil {
		a := thorAddrToEth(*clauses[0].To())
		toAddr = &a
	}

	status := uint64(1)
	if receipt.Reverted {
		status = 0
	}

	logs := flattenLogs(receipt, t, header, txIndex)

	// Derive contract address for CREATE transactions (first clause has no To).
	var contractAddr *common.Address
	if len(clauses) > 0 && clauses[0].To() == nil {
		a := thorAddrToEth(thor.CreateContractAddress(t.ID(), 0, 0))
		contractAddr = &a
	}

	gasPrice := t.MaxFeePerGas()
	if gasPrice == nil {
		gasPrice = new(big.Int)
	}

	return &rpcReceipt{
		TransactionHash:   thorHashToEth(t.ID()),
		TransactionIndex:  hexutil.Uint64(txIndex),
		BlockHash:         thorHashToEth(header.ID()),
		BlockNumber:       hexutil.Uint64(header.Number()),
		From:              thorAddrToEth(origin),
		To:                toAddr,
		CumulativeGasUsed: hexutil.Uint64(receipt.GasUsed),
		GasUsed:           hexutil.Uint64(receipt.GasUsed),
		ContractAddress:   contractAddr,
		Logs:              logs,
		LogsBloom:         zeroBloom,
		Type:              hexutil.Uint64(tx.TypeEthTyped1559),
		Status:            hexutil.Uint64(status),
		EffectiveGasPrice: (*hexutil.Big)(gasPrice),
	}
}

// flattenLogs flattens per-clause events from a VeChain receipt into a flat Ethereum log slice.
func flattenLogs(receipt *tx.Receipt, t *tx.Transaction, header *block.Header, txIndex uint64) []*rpcLog {
	logs := make([]*rpcLog, 0)
	logIndex := uint64(0)
	for _, output := range receipt.Outputs {
		for _, ev := range output.Events {
			topics := make([]common.Hash, len(ev.Topics))
			for i, tp := range ev.Topics {
				topics[i] = thorHashToEth(tp)
			}
			logs = append(logs, &rpcLog{
				Address:          thorAddrToEth(ev.Address),
				Topics:           topics,
				Data:             ev.Data,
				BlockNumber:      hexutil.Uint64(header.Number()),
				TransactionHash:  thorHashToEth(t.ID()),
				TransactionIndex: hexutil.Uint64(txIndex),
				BlockHash:        thorHashToEth(header.ID()),
				LogIndex:         hexutil.Uint64(logIndex),
				Removed:          false,
			})
			logIndex++
		}
	}
	return logs
}

// convertLogDBEvent converts a logdb.Event to an Ethereum rpcLog.
func convertLogDBEvent(ev *logdb.Event, logIndex uint64) *rpcLog {
	topics := make([]common.Hash, 0, 5)
	for _, tp := range ev.Topics {
		if tp == nil {
			break
		}
		topics = append(topics, thorHashToEth(*tp))
	}
	return &rpcLog{
		Address:          thorAddrToEth(ev.Address),
		Topics:           topics,
		Data:             ev.Data,
		BlockNumber:      hexutil.Uint64(ev.BlockNumber),
		TransactionHash:  thorHashToEth(ev.TxID),
		TransactionIndex: hexutil.Uint64(ev.TxIndex),
		BlockHash:        thorHashToEth(ev.BlockID),
		LogIndex:         hexutil.Uint64(logIndex),
		Removed:          false,
	}
}

// ethBlockParamToRevision converts an Ethereum block parameter ("latest", "earliest",
// "pending", "finalized", or a hex block number like "0x1a") to a VeChain revision string.
func ethBlockParamToRevision(param string) string {
	switch strings.ToLower(param) {
	case "", "latest", "pending":
		return "best"
	case "earliest":
		return "0"
	case "finalized":
		return "finalized"
	default:
		// Hex block number like "0x1a" — convert to decimal for ParseRevision.
		if strings.HasPrefix(param, "0x") || strings.HasPrefix(param, "0X") {
			n, err := strconv.ParseUint(param[2:], 16, 64)
			if err == nil {
				return strconv.FormatUint(n, 10)
			}
		}
		return param
	}
}
