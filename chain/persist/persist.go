package persist

import (
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/kv"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/tx"
)

var (
	bestBlockKey = []byte("bestBlock")

	headerPrefix     = []byte("h") // (prefix, hash) -> header
	bodyPrefix       = []byte("b") // (prefix, hash) -> body
	trunkPrefix      = []byte("t") // (prefix, number) -> hash
	txLocationPrefix = []byte("l") // (prefix, txHash) -> txLocation
)

// TxLocation contains information about a tx is settled.
type TxLocation struct {
	BlockHash cry.Hash

	// Index the position of the tx in block's txs.
	Index uint64 // rlp require uint64.
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

// LoadBestBlockHash returns the best block hash on trunk.
func LoadBestBlockHash(r kv.Getter) (*cry.Hash, error) {
	data, err := r.Get(bestBlockKey)
	if err != nil {
		return nil, err
	}
	hash := cry.BytesToHash(data)
	return &hash, nil
}

// SaveBestBlockHash save the best block hash on trunk.
func SaveBestBlockHash(w kv.Putter, hash cry.Hash) error {
	return w.Put(bestBlockKey, hash[:])
}

// LoadRawBlockHeader load block header without being decoded.
func LoadRawBlockHeader(r kv.Getter, hash cry.Hash) (rlp.RawValue, error) {
	return r.Get(append(headerPrefix, hash[:]...))
}

// LoadBlockHeader load decoded block header.
func LoadBlockHeader(r kv.Getter, hash cry.Hash) (*block.Header, error) {
	var header block.Header
	if err := loadRLP(r, append(headerPrefix, hash[:]...), &header); err != nil {
		return nil, err
	}
	return &header, nil
}

// LoadRawBlockBody load block body without being decoded.
func LoadRawBlockBody(r kv.Getter, hash cry.Hash) (rlp.RawValue, error) {
	return r.Get(append(bodyPrefix, hash[:]...))
}

// LoadBlockBody load decoded block body.
func LoadBlockBody(r kv.Getter, hash cry.Hash) (*block.Body, error) {
	var body block.Body
	if err := loadRLP(r, append(bodyPrefix, hash[:]...), &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// SaveBlock save block header and body.
func SaveBlock(w kv.Putter, b *block.Block) error {
	hash := b.Hash()
	if err := saveRLP(w, append(headerPrefix, hash[:]...), b.Header()); err != nil {
		return err
	}
	if err := saveRLP(w, append(bodyPrefix, hash[:]...), b.Body()); err != nil {
		return err
	}
	return nil
}

// SaveTrunkBlockHash save a block's hash on the trunk.
func SaveTrunkBlockHash(w kv.Putter, hash cry.Hash) error {
	// first 4 bytes of block hash present block number
	return w.Put(append(trunkPrefix, hash[:4]...), hash[:])
}

// EraseTrunkBlockHash erase block hash on the trunk.
func EraseTrunkBlockHash(w kv.Putter, hash cry.Hash) error {
	return w.Delete(append(trunkPrefix, hash[:4]...))
}

// LoadTrunkBlockHash returns block's hash with given block number.
func LoadTrunkBlockHash(r kv.Getter, num uint32) (*cry.Hash, error) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], num)
	data, err := r.Get(append(trunkPrefix, n[:]...))
	if err != nil {
		return nil, err
	}
	hash := cry.BytesToHash(data)
	return &hash, nil
}

// SaveTxLocations save locations of all txs in a block.
func SaveTxLocations(w kv.Putter, block *block.Block) error {
	for i, tx := range block.Transactions() {
		loc := TxLocation{
			block.Hash(),
			uint64(i),
		}
		data, err := rlp.EncodeToBytes(&loc)
		if err != nil {
			return err
		}
		txHash := tx.Hash()
		if err := w.Put(append(txLocationPrefix, txHash[:]...), data); err != nil {
			return err
		}
	}
	return nil
}

// EraseTxLocations delete locations of all txs in a block.
func EraseTxLocations(w kv.Putter, block *block.Block) error {
	for _, tx := range block.Transactions() {
		txHash := tx.Hash()
		if err := w.Delete(append(txLocationPrefix, txHash[:]...)); err != nil {
			return err
		}
	}
	return nil
}

// LoadTxLocation load tx location info by tx hash.
func LoadTxLocation(r kv.Getter, txHash cry.Hash) (*TxLocation, error) {
	var loc TxLocation
	if err := loadRLP(r, append(txLocationPrefix, txHash[:]...), &loc); err != nil {
		return nil, err
	}
	return &loc, nil
}

// LoadTx load tx by tx hash.
func LoadTx(r kv.Getter, txHash cry.Hash) (*tx.Transaction, *TxLocation, error) {
	loc, err := LoadTxLocation(r, txHash)
	if err != nil {
		return nil, nil, err
	}
	body, err := LoadBlockBody(r, loc.BlockHash)
	if err != nil {
		return nil, nil, err
	}
	if int(loc.Index) >= len(body.Txs) {
		return nil, nil, errors.New("tx index out of bound")
	}
	return body.Txs[loc.Index], loc, nil
}
