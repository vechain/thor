package kv

// keyspace implements keyspace interface.
type keyspace struct {
	space       string
	spacePrefix string
	store       Store
}

func newKeyspace(space string, store Store) *keyspace {
	return &keyspace{
		space,
		space + "/",
		store,
	}
}

func (ks *keyspace) Space() string {
	return ks.space
}

func (ks *keyspace) makeKey(key []byte) []byte {
	return append([]byte(ks.spacePrefix), key...)
}

func (ks *keyspace) Get(key []byte) (value []byte, err error) {
	return ks.store.Get(ks.makeKey(key))
}

func (ks *keyspace) Has(key []byte) (bool, error) {
	return ks.store.Has(ks.makeKey(key))
}

func (ks *keyspace) Put(key, value []byte) error {
	return ks.store.Put(ks.makeKey(key), value)
}
func (ks *keyspace) Delete(key []byte) error {
	return ks.store.Delete(ks.makeKey(key))
}

func (ks *keyspace) NewBatch() Batch {
	return &tableBatch{
		ks.store.NewBatch(),
		ks.makeKey,
	}
}

func (ks *keyspace) NewIterator(r *Range) Iterator {
	r = r.WithPrefix([]byte(ks.spacePrefix))
	return &tableIter{
		ks.store.NewIterator(r),
		ks.makeKey,
		ks.spacePrefix,
	}
}

func (ks *keyspace) NewKeyspace(space string) Keyspace {
	return newKeyspace(space, ks)
}

////
type tableBatch struct {
	Batch
	makeKey func([]byte) []byte
}

func (b *tableBatch) Put(key, value []byte) error {
	return b.Batch.Put(b.makeKey(key), value)
}

func (b *tableBatch) Delete(key []byte) error {
	return b.Batch.Delete(b.makeKey(key))
}

////
type tableIter struct {
	Iterator
	makeKey func([]byte) []byte
	prefix  string
}

func (i *tableIter) Seek(key []byte) bool {
	return i.Iterator.Seek(i.makeKey(key))
}

func (i *tableIter) Key() []byte {
	return i.Iterator.Key()[len(i.prefix):]
}
