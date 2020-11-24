// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"encoding/binary"
	"math"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	txInfix      = byte(0)
	receiptInfix = byte(1)
	bssSuffix    = byte(2) // backer signatures suffix
)

// BlockSummary presents block summary.
type BlockSummary struct {
	Header    *block.Header
	IndexRoot thor.Bytes32
	Txs       []thor.Bytes32
	Size      uint64
}

// the key for tx/receipt.
// it consists of: ( block id | infix | index )
type txKey [32 + 1 + 8]byte

func makeTxKey(blockID thor.Bytes32, infix byte) (k txKey) {
	copy(k[:], blockID[:])
	k[32] = infix
	return
}

func (k *txKey) SetIndex(i uint64) {
	binary.BigEndian.PutUint64(k[33:], i)
}

func saveRLP(w kv.Putter, key []byte, val interface{}) error {
	data, err := rlp.EncodeToBytes(val)
	if err != nil {
		return err
	}
	return w.Put(key, data)
}

func loadRLP(r kv.Getter, key []byte, val interface{}) error {
	data, err := r.Get(key)
	if err != nil {
		return err
	}
	return rlp.DecodeBytes(data, val)
}

func saveBlockSummary(w kv.Putter, summary *BlockSummary) error {
	return saveRLP(w, summary.Header.ID().Bytes(), summary)
}

func loadBlockSummary(r kv.Getter, id thor.Bytes32) (*BlockSummary, error) {
	var summary BlockSummary
	if err := loadRLP(r, id[:], &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func saveTransaction(w kv.Putter, key txKey, tx *tx.Transaction) error {
	return saveRLP(w, key[:], tx)
}

func loadTransaction(r kv.Getter, key txKey) (*tx.Transaction, error) {
	var tx tx.Transaction
	if err := loadRLP(r, key[:], &tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

func saveReceipt(w kv.Putter, key txKey, receipt *tx.Receipt) error {
	return saveRLP(w, key[:], receipt)
}

func loadReceipt(r kv.Getter, key txKey) (*tx.Receipt, error) {
	var receipt tx.Receipt
	if err := loadRLP(r, key[:], &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func saveBackerSignatures(w kv.Putter, id thor.Bytes32, bss block.ComplexSignatures) error {
	return saveRLP(w, append(id.Bytes(), bssSuffix), bss)
}

func loadBackerSignatures(r kv.Getter, id thor.Bytes32) (block.ComplexSignatures, error) {
	var bss block.ComplexSignatures
	if err := loadRLP(r, append(id.Bytes(), bssSuffix), &bss); err != nil {
		return nil, err
	}
	return bss, nil
}

func branchHeadKey(id thor.Bytes32) []byte {
	return append([]byte("branch"), id.Bytes()...)
}

func saveBranchHead(w kv.Putter, newHead thor.Bytes32, oldHead thor.Bytes32) error {
	if !oldHead.IsZero() {
		if err := w.Delete(branchHeadKey(oldHead)); err != nil {
			return err
		}
	}
	w.Put(branchHeadKey(newHead), []byte{})
	return nil
}

func loadBranchHeads(s kv.Store, minNum uint32) (heads []thor.Bytes32) {
	// rng := kv.Range(*util.BytesPrefix(append([]byte("branch"))))
	// s.Iterate(rng, func(pair kv.Pair) bool {
	// 	id := thor.BytesToBytes32(pair.Key()[6:])
	// 	if block.Number(id) >= minNum {
	// 		heads = append(heads, id)
	// 	}
	// 	return true
	// })

	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, minNum)
	start := append([]byte("branch"), b...)
	binary.BigEndian.PutUint32(b, uint32(math.Exp2(32)-1))
	limit := append([]byte("branch"), b...)

	s.Iterate(kv.Range{Start: start, Limit: limit}, func(pair kv.Pair) bool {
		id := thor.BytesToBytes32(pair.Key()[6:])
		heads = append(heads, id)
		return true
	})

	return heads
}

func isBranchHead(r kv.Getter, id thor.Bytes32) bool {
	_, err := r.Get(branchHeadKey(id))
	return err == nil
}
