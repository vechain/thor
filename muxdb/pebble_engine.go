package muxdb

import (
	"context"

	"github.com/cockroachdb/pebble"
	"github.com/vechain/thor/kv"
)

type pebbleEngine struct {
	db *pebble.DB
}

// newPebbleEngine wraps pebble db to comply engine interface.
func newPebbleEngine(db *pebble.DB) engine {
	return &pebbleEngine{db}
}

func (pe *pebbleEngine) Close() error {
	return pe.db.Close()
}

func (pe *pebbleEngine) IsNotFound(err error) bool {
	return err == pebble.ErrNotFound
}

func (pe *pebbleEngine) Get(key []byte) ([]byte, error) {
	v, c, err := pe.db.Get(key)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return append([]byte(nil), v...), nil
}

func (pe *pebbleEngine) Has(key []byte) (bool, error) {
	_, c, err := pe.db.Get(key)
	if err != nil {
		return false, err
	}
	defer c.Close()
	return true, nil
}

func (pe *pebbleEngine) Put(key, val []byte) error {
	return pe.db.Set(key, val, pebble.NoSync)
}

func (pe *pebbleEngine) Delete(key []byte) error {
	return pe.db.Delete(key, pebble.NoSync)
}

func (pe *pebbleEngine) DeleteRange(ctx context.Context, rng kv.Range) (int, error) {
	if err := pe.db.DeleteRange(rng.Start, rng.Limit, pebble.NoSync); err != nil {
		return 0, err
	}
	return -1, nil
}

func (pe *pebbleEngine) Snapshot(fn func(kv.Getter) error) error {
	s := pe.db.NewSnapshot()
	defer s.Close()

	return fn(&struct {
		kv.GetFunc
		kv.HasFunc
	}{
		func(key []byte) ([]byte, error) {
			v, c, err := s.Get(key)
			if err != nil {
				return nil, err
			}
			defer c.Close()
			return append([]byte(nil), v...), nil
		},
		func(key []byte) (bool, error) {
			_, c, err := s.Get(key)
			if err != nil {
				return false, err
			}
			defer c.Close()
			return true, nil
		},
	})
}

func (pe *pebbleEngine) Batch(fn func(kv.PutFlusher) error) error {
	b := pe.db.NewBatch()
	defer b.Close()

	if err := fn(&struct {
		kv.PutFunc
		kv.DeleteFunc
		kv.FlushFunc
	}{
		// put
		func(key, val []byte) error {
			b.Set(key, val, pebble.NoSync)
			return nil
		},
		// delete
		func(key []byte) error {
			b.Delete(key, pebble.NoSync)
			return nil
		},
		// flush
		func() error {
			if b.Count() == 0 {
				return nil
			}
			defer b.Reset()
			return b.Commit(pebble.NoSync)
		},
	}); err != nil {
		return err
	}
	if b.Count() == 0 {
		return nil
	}
	return b.Commit(pebble.NoSync)
}

func (pe *pebbleEngine) Iterate(rng kv.Range, fn func(kv.Pair) bool) error {
	it := pe.db.NewIter(&pebble.IterOptions{LowerBound: rng.Start, UpperBound: rng.Limit})
	defer it.Close()

	for it.First(); it.Valid(); it.Next() {
		if !fn(it) {
			break
		}
	}
	return it.Error()
}
