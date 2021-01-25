// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"encoding/binary"
	"io"

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

// the key for tx/receipt.
// it consists of: ( block id | infix | index )
type txKey [32 + 1 + 8]byte

// BlockSummary presents block summary.
type BlockSummary struct {
	Header    *block.Header
	IndexRoot thor.Bytes32
	Txs       []thor.Bytes32
	Size      uint64
	Beta      Beta
}

// Beta represents beta from block signer's VRF.
type Beta []byte
type _beta Beta

// EncodeRLP implements rlp.Encoder.
func (b *Beta) EncodeRLP(w io.Writer) error {
	if len(*b) == 0 {
		return nil
	}

	return rlp.Encode(w, (*_beta)(b))
}

// DecodeRLP implements rlp.Decoder.
func (b *Beta) DecodeRLP(s *rlp.Stream) error {
	var obj _beta
	if err := s.Decode(&obj); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*b = Beta{}
			return nil
		}
		return err
	}

	*b = Beta(obj)
	return nil
}

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
