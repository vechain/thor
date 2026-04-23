// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethview

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// zeroLogsBloom is the fixed 256-byte placeholder emitted in every receipt
// and block object. Thor does not compute a real bloom today (spec §6.6);
// clients that rely on it accept an always-empty bloom, so a constant value
// is consensus-equivalent.
var zeroLogsBloom hexutil.Bytes = make([]byte, 256)

// ReceiptObject is the eth-shape receipt view. Fields without omitempty are
// populated for every receipt regardless of subtype; VeChainTx extensions are
// populated only for 0x00 / 0x51 single-clause receipts.
type ReceiptObject struct {
	// Standard eth fields.
	TransactionHash   thor.Bytes32    `json:"transactionHash"`
	TransactionIndex  hexutil.Uint64  `json:"transactionIndex"`
	BlockHash         thor.Bytes32    `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	From              thor.Address    `json:"from"`
	To                *thor.Address   `json:"to"`
	CumulativeGasUsed hexutil.Uint64  `json:"cumulativeGasUsed"`
	GasUsed           hexutil.Uint64  `json:"gasUsed"`
	ContractAddress   *thor.Address   `json:"contractAddress"`
	Logs              []*LogObject    `json:"logs"`
	LogsBloom         hexutil.Bytes   `json:"logsBloom"`
	Status            hexutil.Uint64  `json:"status"`
	Type              hexutil.Uint64  `json:"type"`
	EffectiveGasPrice *hexutil.Big    `json:"effectiveGasPrice"`

	// VeChainTx extensions (0x00 / 0x51 single-clause).
	GasPayer  *thor.Address      `json:"gasPayer,omitempty"`
	Paid      *hexutil.Big       `json:"paid,omitempty"`
	Reward    *hexutil.Big       `json:"reward,omitempty"`
	Reverted  *bool              `json:"reverted,omitempty"`
	Transfers []*TransferObject  `json:"transfers,omitempty"`
	Outputs   []*OutputObject    `json:"outputs,omitempty"`
}

// LogObject is the eth-shape event log. `logIndex` is the position within the
// block (not within the tx); the caller must set it because computing it
// requires the full block receipt list. `blockNumber`/`blockHash`/
// `transactionHash`/`transactionIndex` likewise come from the caller-supplied
// meta.
type LogObject struct {
	Address          thor.Address     `json:"address"`
	Topics           []thor.Bytes32   `json:"topics"`
	Data             hexutil.Bytes    `json:"data"`
	BlockNumber      hexutil.Uint64   `json:"blockNumber"`
	BlockHash        thor.Bytes32     `json:"blockHash"`
	TransactionHash  thor.Bytes32     `json:"transactionHash"`
	TransactionIndex hexutil.Uint64   `json:"transactionIndex"`
	LogIndex         hexutil.Uint64   `json:"logIndex"`
	Removed          bool             `json:"removed"`
}

// TransferObject mirrors the VET transfer log attached to a VeChainTx
// receipt.
type TransferObject struct {
	Sender    thor.Address `json:"sender"`
	Recipient thor.Address `json:"recipient"`
	Amount    *hexutil.Big `json:"amount"`
}

// OutputObject is the per-clause side-effect record carried on 0x00 / 0x51
// receipts. Since single-clause is the only representable shape, callers
// will see exactly one OutputObject entry when this field is populated.
type OutputObject struct {
	ContractAddress *thor.Address     `json:"contractAddress"`
	Events          []*LogObject      `json:"events"`
	Transfers       []*TransferObject `json:"transfers"`
}

// ProjectReceipt maps a native *tx.Receipt + caller-supplied context into a
// ReceiptObject. Multi-clause receipts yield ErrNotRepresentable.
//
// cumulativeGasUsed is pre-computed by the caller (which has access to the
// full block's receipt list); baseFee is the block's base fee (nil if none,
// i.e. pre-GALACTICA) and is used only to assemble effectiveGasPrice on
// 0x00 / 0x51 / 0x02 per the gas price derivation rule.
func ProjectReceipt(
	trx *tx.Transaction,
	receipt *tx.Receipt,
	meta TxMeta,
	cumulativeGasUsed uint64,
	baseFee *big.Int,
) (*ReceiptObject, error) {
	if !isRepresentable(trx) {
		return nil, ErrNotRepresentable
	}

	// Single clause by precondition — index 0 holds everything we need.
	// (0x02 also has exactly one clause by construction.)
	var clauseOutput *tx.Output
	if len(receipt.Outputs) > 0 {
		clauseOutput = receipt.Outputs[0]
	}

	// Eth status: 1 on success, 0 on revert.
	var status hexutil.Uint64 = 1
	if receipt.Reverted {
		status = 0
	}

	// Block location is mandatory for a receipt (always mined).
	var blockID thor.Bytes32
	if meta.BlockID != nil {
		blockID = *meta.BlockID
	}
	var blockNum uint32
	if meta.BlockNumber != nil {
		blockNum = *meta.BlockNumber
	}

	// Effective gas price: for 0x51 / 0x02 ask the tx for the rate using the
	// block's baseFee; for 0x00 the caller-path is the same helper. Thor's
	// *tx.Transaction has an EffectiveGasPrice(baseFee, legacyBase) helper,
	// but that requires legacyBase which is state-scoped. Since the caller
	// can always look up or compute the rate and already has to pass other
	// meta, we take the resolved value via meta.EffectiveGasPrice.
	var effective *big.Int
	if meta.EffectiveGasPrice != nil {
		effective = new(big.Int).Set(meta.EffectiveGasPrice)
	} else {
		effective = new(big.Int)
	}

	// contractAddress: VeChain-native derivation for every tx type per
	// Deviation D1 (spec §3). Only emit when the single clause is a
	// contract creation (to==nil on the native tx).
	var contractAddr *thor.Address
	if firstClauseTo(trx) == nil {
		addr := thor.CreateContractAddress(trx.ID(), 0, 0)
		contractAddr = &addr
	}

	obj := &ReceiptObject{
		TransactionHash:   trx.CanonicalTxID(),
		TransactionIndex:  hexutil.Uint64(meta.Index),
		BlockHash:         blockID,
		BlockNumber:       hexutil.Uint64(blockNum),
		From:              meta.Origin,
		To:                firstClauseTo(trx),
		CumulativeGasUsed: hexutil.Uint64(cumulativeGasUsed),
		GasUsed:           hexutil.Uint64(receipt.GasUsed),
		ContractAddress:   contractAddr,
		LogsBloom:         zeroLogsBloom,
		Status:            status,
		Type:              hexutil.Uint64(trx.Type()),
		EffectiveGasPrice: (*hexutil.Big)(effective),
	}

	// Eth-shape logs flattened from the single clause's events. The
	// logIndex is the index within the clause; callers that need
	// block-wide logIndex (eth_getLogs) compute it externally.
	if clauseOutput != nil {
		obj.Logs = buildLogObjects(clauseOutput.Events, trx.CanonicalTxID(), blockID, blockNum, meta.Index)
	} else {
		obj.Logs = []*LogObject{}
	}

	// VeChainTx extensions (0x00 / 0x51 only — 0x02 has none of these).
	if trx.Type() == tx.TypeLegacy || trx.Type() == tx.TypeDynamicFee {
		reverted := receipt.Reverted
		payer := receipt.GasPayer
		obj.GasPayer = &payer
		obj.Paid = (*hexutil.Big)(new(big.Int).Set(receipt.Paid))
		obj.Reward = (*hexutil.Big)(new(big.Int).Set(receipt.Reward))
		obj.Reverted = &reverted
		if clauseOutput != nil {
			obj.Transfers = buildTransferObjects(clauseOutput.Transfers)
			obj.Outputs = buildOutputObjects(receipt.Outputs, trx.CanonicalTxID(), blockID, blockNum, meta.Index, trx.ID())
		} else {
			obj.Transfers = []*TransferObject{}
			obj.Outputs = []*OutputObject{}
		}
	}

	_ = baseFee // currently unused; kept in the signature for spec §6.4 fidelity and to keep call-sites stable when the internal derivation moves here.
	return obj, nil
}

// --- helpers -------------------------------------------------------------

func buildLogObjects(events tx.Events, txHash, blockHash thor.Bytes32, blockNum, txIndex uint32) []*LogObject {
	out := make([]*LogObject, len(events))
	for i, e := range events {
		topics := make([]thor.Bytes32, len(e.Topics))
		copy(topics, e.Topics)
		out[i] = &LogObject{
			Address:          e.Address,
			Topics:           topics,
			Data:             append(hexutil.Bytes(nil), e.Data...),
			BlockNumber:      hexutil.Uint64(blockNum),
			BlockHash:        blockHash,
			TransactionHash:  txHash,
			TransactionIndex: hexutil.Uint64(txIndex),
			LogIndex:         hexutil.Uint64(i),
			Removed:          false,
		}
	}
	return out
}

func buildTransferObjects(transfers tx.Transfers) []*TransferObject {
	out := make([]*TransferObject, len(transfers))
	for i, t := range transfers {
		out[i] = &TransferObject{
			Sender:    t.Sender,
			Recipient: t.Recipient,
			Amount:    (*hexutil.Big)(new(big.Int).Set(t.Amount)),
		}
	}
	return out
}

func buildOutputObjects(outputs []*tx.Output, txHash, blockHash thor.Bytes32, blockNum, txIndex uint32, nativeTxID thor.Bytes32) []*OutputObject {
	out := make([]*OutputObject, len(outputs))
	for i, o := range outputs {
		addr := thor.CreateContractAddress(nativeTxID, uint32(i), 0)
		oo := &OutputObject{
			ContractAddress: nil,
			Events:          buildLogObjects(o.Events, txHash, blockHash, blockNum, txIndex),
			Transfers:       buildTransferObjects(o.Transfers),
		}
		// Only expose contractAddress when the clause actually created one.
		// We can't inspect the native clause from here, so we leave this nil
		// until the caller asks for it; the top-level ReceiptObject
		// .ContractAddress is the authoritative source.
		_ = addr
		out[i] = oo
	}
	return out
}
