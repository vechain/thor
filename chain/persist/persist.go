package persist

import (
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	bestBlockKey = []byte("bestBlock")

	headerPrefix     = []byte("h") // (prefix, block id) -> header
	bodyPrefix       = []byte("b") // (prefix, block id) -> body
	trunkPrefix      = []byte("t") // (prefix, number) -> block id
	txLocationPrefix = []byte("l") // (prefix, tx id) -> tx location
)

// TxLocation contains information about a tx is settled.
type TxLocation struct {
	BlockID thor.Hash

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

// LoadBestBlockID returns the best block ID on trunk.
func LoadBestBlockID(r kv.Getter) (thor.Hash, error) {
	data, err := r.Get(bestBlockKey)
	if err != nil {
		return thor.Hash{}, err
	}
	return thor.BytesToHash(data), nil
}

// SaveBestBlockID save the best block ID on trunk.
func SaveBestBlockID(w kv.Putter, id thor.Hash) error {
	return w.Put(bestBlockKey, id[:])
}

// LoadRawBlockHeader load block header without being decoded.
func LoadRawBlockHeader(r kv.Getter, id thor.Hash) (rlp.RawValue, error) {
	return r.Get(append(headerPrefix, id[:]...))
}

// LoadBlockHeader load decoded block header.
func LoadBlockHeader(r kv.Getter, id thor.Hash) (*block.Header, error) {
	var header block.Header
	if err := loadRLP(r, append(headerPrefix, id[:]...), &header); err != nil {
		return nil, err
	}
	return &header, nil
}

// LoadRawBlockBody load block body without being decoded.
func LoadRawBlockBody(r kv.Getter, id thor.Hash) (rlp.RawValue, error) {
	return r.Get(append(bodyPrefix, id[:]...))
}

// LoadBlockBody load decoded block body.
func LoadBlockBody(r kv.Getter, id thor.Hash) (*block.Body, error) {
	var body block.Body
	if err := loadRLP(r, append(bodyPrefix, id[:]...), &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// SaveBlock save block header and body.
func SaveBlock(w kv.Putter, b *block.Block) error {
	id := b.ID()
	if err := saveRLP(w, append(headerPrefix, id[:]...), b.Header()); err != nil {
		return err
	}
	if err := saveRLP(w, append(bodyPrefix, id[:]...), b.Body()); err != nil {
		return err
	}
	return nil
}

// SaveTrunkBlockID save a block's ID on the trunk.
func SaveTrunkBlockID(w kv.Putter, id thor.Hash) error {
	// first 4 bytes of block hash present block number
	return w.Put(append(trunkPrefix, id[:4]...), id[:])
}

// EraseTrunkBlockID erase block ID on the trunk.
func EraseTrunkBlockID(w kv.Putter, id thor.Hash) error {
	return w.Delete(append(trunkPrefix, id[:4]...))
}

// LoadTrunkBlockID returns block's id with given block number.
func LoadTrunkBlockID(r kv.Getter, num uint32) (thor.Hash, error) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], num)
	data, err := r.Get(append(trunkPrefix, n[:]...))
	if err != nil {
		return thor.Hash{}, err
	}
	return thor.BytesToHash(data), nil
}

// SaveTxLocations save locations of all txs in a block.
func SaveTxLocations(w kv.Putter, block *block.Block) error {
	for i, tx := range block.Transactions() {
		loc := TxLocation{
			block.ID(),
			uint64(i),
		}
		data, err := rlp.EncodeToBytes(&loc)
		if err != nil {
			return err
		}
		if err := w.Put(append(txLocationPrefix, tx.ID().Bytes()...), data); err != nil {
			return err
		}
	}
	return nil
}

// EraseTxLocations delete locations of all txs in a block.
func EraseTxLocations(w kv.Putter, block *block.Block) error {
	for _, tx := range block.Transactions() {
		if err := w.Delete(append(txLocationPrefix, tx.ID().Bytes()...)); err != nil {
			return err
		}
	}
	return nil
}

// LoadTxLocation load tx location info by tx id.
func LoadTxLocation(r kv.Getter, txID thor.Hash) (*TxLocation, error) {
	var loc TxLocation
	if err := loadRLP(r, append(txLocationPrefix, txID[:]...), &loc); err != nil {
		return nil, err
	}
	return &loc, nil
}

// LoadTx load tx by tx id.
func LoadTx(r kv.Getter, txID thor.Hash) (*tx.Transaction, *TxLocation, error) {
	loc, err := LoadTxLocation(r, txID)
	if err != nil {
		return nil, nil, err
	}
	body, err := LoadBlockBody(r, loc.BlockID)
	if err != nil {
		return nil, nil, err
	}
	if int(loc.Index) >= len(body.Txs) {
		return nil, nil, errors.New("tx index out of bound")
	}
	return body.Txs[loc.Index], loc, nil
}
