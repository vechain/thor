// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// ResolveBlockTag maps an Ethereum block tag, hex block number, or block hash to
// a block summary in the canonical chain. The returned summary carries the
// versioned trie.Root needed for correct state access — always use summary.Root()
// rather than trie.Root{Hash: header.StateRoot()} when opening a state.
//
// Supported tags: "latest", "earliest", "pending", "safe", "finalized".
// Numeric strings: "0x1" → block number 1.
// Hash strings (66 chars, "0x" + 64 hex digits): resolved directly by hash.
//
// "pending", "safe", and "finalized" are treated as "latest" in Phase 1.
func ResolveBlockTag(tag string, repo *chain.Repository) (*chain.BlockSummary, error) {
	switch strings.ToLower(tag) {
	case "", "latest", "pending", "safe", "finalized":
		// NOTE: "pending" returns confirmed state. Full pool scanning is not implemented.
		return repo.BestBlockSummary(), nil
	case "earliest":
		id := repo.GenesisBlock().Header().ID()
		return repo.GetBlockSummary(id)
	}

	// 32-byte hash (0x + 64 hex chars = 66 chars)
	if strings.HasPrefix(tag, "0x") && len(tag) == 66 {
		var id thor.Bytes32
		b, err := hex.DecodeString(tag[2:])
		if err != nil {
			return nil, fmt.Errorf("invalid block hash %q: %w", tag, err)
		}
		copy(id[:], b)
		summary, err := repo.GetBlockSummary(id)
		if err != nil {
			return nil, fmt.Errorf("block not found: %w", err)
		}
		return summary, nil
	}

	// Hex block number
	if strings.HasPrefix(tag, "0x") {
		n, err := strconv.ParseUint(tag[2:], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid block number %q: %w", tag, err)
		}
		summary, err := repo.NewBestChain().GetBlockSummary(uint32(n))
		if err != nil {
			return nil, fmt.Errorf("block %d not found: %w", n, err)
		}
		return summary, nil
	}

	return nil, fmt.Errorf("unsupported block tag %q", tag)
}

// StateAt opens the state at the block identified by tag.
func StateAt(tag string, repo *chain.Repository, stater *state.Stater) (*state.State, error) {
	summary, err := ResolveBlockTag(tag, repo)
	if err != nil {
		return nil, err
	}
	return stater.NewState(summary.Root()), nil
}

// BuildEthBlock constructs an EthBlock from a VeChain block header.
// Only TypeEthTyped1559 transactions are included in the transactions field.
func BuildEthBlock(
	header *block.Header,
	repo *chain.Repository,
	chainID uint64,
	fullTxs bool,
) (*EthBlock, error) {
	blk, err := repo.GetBlock(header.ID())
	if err != nil {
		return nil, err
	}
	receipts, err := repo.GetBlockReceipts(header.ID())
	if err != nil {
		return nil, err
	}

	txs := blk.Transactions()
	blockHash := common.Hash(header.ID())
	blockNum := uint64(header.Number())

	var (
		ethTxHashes    []common.Hash
		ethTxFull      []*EthTx
		ethGasUsed     uint64
		ethProjIdx     uint64
		logOffset      uint64
		ethTxsForRoot  []*tx.Transaction
		ethRecsForRoot []*EthReceipt
		blockBloom     [256]byte
	)

	baseFee := header.BaseFee()

	for i, t := range txs {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		ethGasUsed += receipts[i].GasUsed

		rec := ToEthReceipt(t, receipts[i], chainID, blockHash, blockNum, ethProjIdx, ethGasUsed, logOffset, baseFee)
		logOffset += uint64(len(rec.Logs))

		// OR this receipt's bloom into the block-level bloom.
		for j, b := range rec.LogsBloom {
			blockBloom[j] |= b
		}

		ethTxsForRoot = append(ethTxsForRoot, t)
		ethRecsForRoot = append(ethRecsForRoot, rec)

		if fullTxs {
			ethTxFull = append(ethTxFull, ToEthTx(t, chainID, blockHash, blockNum, ethProjIdx, baseFee))
		} else {
			ethTxHashes = append(ethTxHashes, common.Hash(t.ID()))
		}
		ethProjIdx++
	}

	var transactions any
	if fullTxs {
		if ethTxFull == nil {
			ethTxFull = []*EthTx{}
		}
		transactions = ethTxFull
	} else {
		if ethTxHashes == nil {
			ethTxHashes = []common.Hash{}
		}
		transactions = ethTxHashes
	}

	var baseFeePerGas *hexutil.Big
	if baseFee != nil {
		baseFeePerGas = (*hexutil.Big)(baseFee)
	}

	return &EthBlock{
		Number:           hexutil.Uint64(blockNum),
		Hash:             blockHash,
		ParentHash:       common.Hash(header.ParentID()),
		Nonce:            zeroNonce,
		Sha3Uncles:       emptyUncleHash,
		LogsBloom:        blockBloom[:],
		TransactionsRoot: ethTransactionsRoot(ethTxsForRoot),
		StateRoot:        common.Hash(header.StateRoot()),
		ReceiptsRoot:     ethReceiptsRoot(ethRecsForRoot),
		Miner:            common.Address(header.Beneficiary()),
		ExtraData:        []byte{},
		Size:             hexutil.Uint64(blk.Size()),
		GasLimit:         hexutil.Uint64(header.GasLimit()),
		GasUsed:          hexutil.Uint64(ethGasUsed),
		Timestamp:        hexutil.Uint64(header.Timestamp()),
		BaseFeePerGas:    baseFeePerGas,
		Transactions:     transactions,
		Uncles:           []common.Hash{},
	}, nil
}

// rlpLogEntry is the consensus RLP encoding of an event log: only address, topics, data.
type rlpLogEntry struct {
	Address common.Address
	Topics  []common.Hash
	Data    []byte
}

// rlpReceiptBody is the consensus encoding of an EIP-1559 (type 2) receipt.
type rlpReceiptBody struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	Bloom             [256]byte
	Logs              []rlpLogEntry
}

// ethReceiptWireBytes encodes an EthReceipt as the EIP-2718 type-2 consensus bytes:
// 0x02 || RLP(status, cumulativeGasUsed, bloom, logs).
func ethReceiptWireBytes(rec *EthReceipt) []byte {
	status := []byte{0x01}
	if rec.Status == 0 {
		status = []byte{}
	}

	var bloom [256]byte
	copy(bloom[:], rec.LogsBloom)

	logs := make([]rlpLogEntry, len(rec.Logs))
	for i, log := range rec.Logs {
		logs[i] = rlpLogEntry{Address: log.Address, Topics: log.Topics, Data: log.Data}
	}

	body := rlpReceiptBody{
		PostStateOrStatus: status,
		CumulativeGasUsed: uint64(rec.CumulativeGasUsed),
		Bloom:             bloom,
		Logs:              logs,
	}

	var buf bytes.Buffer
	buf.WriteByte(0x02) // EIP-1559 receipt type byte
	if err := rlp.Encode(&buf, body); err != nil {
		panic(err) // only fails on unencodable types, which rlpReceiptBody is not
	}
	return buf.Bytes()
}

// ethTxDerivableList wraps []*tx.Transaction for use with ethtypes.DeriveSha.
// GetRlp returns the EIP-2718 wire bytes (0x02 || RLP body) for each tx.
type ethTxDerivableList []*tx.Transaction

func (l ethTxDerivableList) Len() int { return len(l) }
func (l ethTxDerivableList) GetRlp(i int) []byte {
	b, err := l[i].MarshalBinary()
	if err != nil {
		panic(err)
	}
	return b
}

// ethReceiptDerivableList wraps []*EthReceipt for use with ethtypes.DeriveSha.
type ethReceiptDerivableList []*EthReceipt

func (l ethReceiptDerivableList) Len() int            { return len(l) }
func (l ethReceiptDerivableList) GetRlp(i int) []byte { return ethReceiptWireBytes(l[i]) }

// ethTransactionsRoot computes the Ethereum Keccak256 MPT root over the EIP-1559
// encoded wire bytes of the given ETH transactions (projected tx index as trie key).
func ethTransactionsRoot(txs []*tx.Transaction) common.Hash {
	return ethtypes.DeriveSha(ethTxDerivableList(txs))
}

// ethReceiptsRoot computes the Ethereum Keccak256 MPT root over the EIP-1559
// encoded consensus bytes of the given ETH receipts.
func ethReceiptsRoot(recs []*EthReceipt) common.Hash {
	return ethtypes.DeriveSha(ethReceiptDerivableList(recs))
}

// ProjectedEthIndex returns the 0-based Ethereum transaction index for a TypeEthTyped1559 tx.
// canonicalIdx is the tx's position counting all tx types in the block.
func ProjectedEthIndex(receipts tx.Receipts, canonicalIdx uint64) uint64 {
	var count uint64
	for i := range canonicalIdx {
		if receipts[i].Type == tx.TypeEthDynamicFee {
			count++
		}
	}
	return count
}

// CumulativeEthGasUsed returns the cumulative gas used by TypeEthTyped1559 transactions
// up to and including the tx at canonicalIdx.
func CumulativeEthGasUsed(receipts tx.Receipts, canonicalIdx uint64) uint64 {
	var total uint64
	for i := uint64(0); i <= canonicalIdx; i++ {
		if receipts[i].Type == tx.TypeEthDynamicFee {
			total += receipts[i].GasUsed
		}
	}
	return total
}

// EthLogOffset returns the number of logs emitted by TypeEthTyped1559 transactions
// strictly before canonicalIdx (used as the starting logIndex for a tx's logs).
func EthLogOffset(receipts tx.Receipts, canonicalIdx uint64) uint64 {
	var offset uint64
	for i := range canonicalIdx {
		if receipts[i].Type == tx.TypeEthDynamicFee && len(receipts[i].Outputs) > 0 {
			offset += uint64(len(receipts[i].Outputs[0].Events))
		}
	}
	return offset
}

// ParseBytes32Compact parses a 0x-prefixed hex string of variable length into a
// right-aligned Bytes32. Unlike thor.ParseBytes32, it accepts compact Ethereum
// encoding such as "0x0" for storage slot 0.
func ParseBytes32Compact(s string) (thor.Bytes32, error) {
	if len(s) < 2 || s[0] != '0' || (s[1] != 'x' && s[1] != 'X') {
		return thor.Bytes32{}, fmt.Errorf("invalid hex %q", s)
	}
	raw := s[2:]
	if len(raw)%2 != 0 {
		raw = "0" + raw
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return thor.Bytes32{}, fmt.Errorf("invalid hex %q: %w", s, err)
	}
	if len(b) > 32 {
		return thor.Bytes32{}, fmt.Errorf("hex value too long for bytes32 %q", s)
	}
	var h32 thor.Bytes32
	copy(h32[32-len(b):], b)
	return h32, nil
}
