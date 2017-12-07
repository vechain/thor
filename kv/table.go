package kv

// Table introduces table concept for a kv store.
type Table interface {
	Name() string

	Duplex

	NewBatch() Batch
	NewIterator(r *Range) Iterator
}

// table implements Table interface.
type table struct {
	name  string
	store Store
}

func (t *table) Name() string {
	return t.name
}

func (t *table) makeKey(key []byte) []byte {
	return append([]byte(t.name), key...)
}

func (t *table) Get(key []byte) (value []byte, err error) {
	return t.store.Get(t.makeKey(key))
}

func (t *table) Has(key []byte) (bool, error) {
	return t.store.Has(t.makeKey(key))
}

func (t *table) Put(key, value []byte) error {
	return t.store.Put(t.makeKey(key), value)
}
func (t *table) Delete(key []byte) error {
	return t.store.Delete(t.makeKey(key))
}

func (t *table) NewBatch() Batch {
	return &tableBatch{
		t.NewBatch(),
		t.makeKey,
	}
}

func (t *table) NewIterator(r *Range) Iterator {
	r = r.WithPrefix([]byte(t.name))
	return &tableIter{
		t.store.NewIterator(r),
		t.makeKey,
		t.name,
	}
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
	name    string
}

func (i *tableIter) Seek(key []byte) bool {
	return i.Iterator.Seek(i.makeKey(key))
}

func (i *tableIter) Key() []byte {
	return i.Iterator.Key()[len(i.name):]
}
