package persist

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	bestBlockKey = []byte("bestBlock")

	blockPrefix         = []byte("b") // (prefix, block id) -> block
	trunkPrefix         = []byte("t") // (prefix, number) -> block id
	txLocationPrefix    = []byte("l") // (prefix, tx id) -> tx location
	blockReceiptsPrefix = []byte("r") // (prefix, block id) -> receipts
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

// LoadRawBlock load rlp encoded raw block.
func LoadRawBlock(r kv.Getter, id thor.Hash) (block.Raw, error) {
	return r.Get(append(blockPrefix, id[:]...))
}

// SaveBlock encode block and save in db.
func SaveBlock(w kv.Putter, b *block.Block) (block.Raw, error) {
	data, err := rlp.EncodeToBytes(b)
	if err != nil {
		return nil, err
	}
	if err := w.Put(append(blockPrefix, b.Header().ID().Bytes()...), data); err != nil {
		return nil, err
	}
	return block.Raw(data), nil
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
func SaveTxLocations(w kv.Putter, txs tx.Transactions, blockID thor.Hash) error {
	for i, tx := range txs {
		loc := TxLocation{
			blockID,
			uint64(i),
		}
		if err := saveRLP(w, append(txLocationPrefix, tx.ID().Bytes()...), &loc); err != nil {
			return err
		}
	}
	return nil
}

// SaveBlockReceipts save tx receipts of a block.
func SaveBlockReceipts(w kv.Putter, blockID thor.Hash, receipts tx.Receipts) error {
	return saveRLP(w, append(blockReceiptsPrefix, blockID[:]...), receipts)
}

// LoadBlockReceipts load tx receipts of a block.
func LoadBlockReceipts(r kv.Getter, blockID thor.Hash) (tx.Receipts, error) {
	var receipts tx.Receipts
	if err := loadRLP(r, append(blockReceiptsPrefix, blockID[:]...), &receipts); err != nil {
		return nil, err
	}
	return receipts, nil
}

// EraseTxLocations delete locations of all txs in a block.
func EraseTxLocations(w kv.Putter, txs tx.Transactions) error {
	for _, tx := range txs {
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
