package state

import (
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/vechain/thor/kv"
)

func newTrieDatabase(kv kv.GetPutter) *trie.Database {
	return trie.NewDatabase(&ethDatabase{kv})
}

// implements ethdb.Database
type ethDatabase struct {
	kv kv.GetPutter
}

func (db *ethDatabase) Put(key []byte, value []byte) error {
	return db.kv.Put(key, value)
}

func (db *ethDatabase) Get(key []byte) ([]byte, error) {
	return db.kv.Get(key)
}
func (db *ethDatabase) Has(key []byte) (bool, error) {
	return db.kv.Has(key)
}
func (db *ethDatabase) Delete(key []byte) error {
	return db.kv.Delete(key)
}

func (db *ethDatabase) Close() {
	panic("never")
}

func (db *ethDatabase) NewBatch() ethdb.Batch {
	return &ethBatch{
		db.kv,
		db.kv.NewBatch(),
		0,
	}
}

type ethBatch struct {
	kv    kv.GetPutter
	batch kv.Batch
	size  int
}

func (b *ethBatch) Put(key []byte, value []byte) error {
	b.size += len(value)
	return b.batch.Put(key, value)
}

func (b *ethBatch) ValueSize() int {
	return b.size
}
func (b *ethBatch) Write() error {
	return b.batch.Write()
}

func (b *ethBatch) Reset() {
	b.batch = b.kv.NewBatch()
	b.size = 0
}
