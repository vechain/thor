// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	// Use suffix here to let header, txs and receipts of a block be adjacent in kv store,
	// to optimize access time.
	bodySuffix     = byte('b')
	receiptsSuffix = byte('r')
)

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

type extendedHeader struct {
	Header    *block.Header
	IndexRoot thor.Bytes32
}

func saveBlockHeader(w kv.Putter, header *extendedHeader) error {
	return saveRLP(w, header.Header.ID().Bytes(), header)
}

func loadBlockHeader(r kv.Getter, id thor.Bytes32) (*extendedHeader, error) {
	var header extendedHeader
	if err := loadRLP(r, id[:], &header); err != nil {
		return nil, err
	}
	return &header, nil
}

func saveTransactions(w kv.Putter, id thor.Bytes32, txs tx.Transactions) error {
	return saveRLP(w, append(id.Bytes(), bodySuffix), txs)
}

func loadTransactions(r kv.Getter, id thor.Bytes32) (tx.Transactions, error) {
	var txs tx.Transactions
	if err := loadRLP(r, append(id.Bytes(), bodySuffix), &txs); err != nil {
		return nil, err
	}
	return txs, nil
}

func saveReceipts(w kv.Putter, id thor.Bytes32, receipts tx.Receipts) error {
	return saveRLP(w, append(id.Bytes(), receiptsSuffix), receipts)
}

func loadReceipts(r kv.Getter, id thor.Bytes32) (tx.Receipts, error) {
	var receipts tx.Receipts
	if err := loadRLP(r, append(id.Bytes(), receiptsSuffix), &receipts); err != nil {
		return nil, err
	}
	return receipts, nil
}
