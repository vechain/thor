package txpool

import (
	Sort "sort"
	"sync"

	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

type entry struct {
	lock    sync.Mutex
	dirty   bool
	all     *cache.RandCache
	pending txObjects
	sorted  bool
}

func newEntry(size int) *entry {
	return &entry{all: cache.NewRandCache(size)}
}

func (e *entry) find(id thor.Bytes32) *txObject {
	e.lock.Lock()
	defer e.lock.Unlock()

	if value, ok := e.all.Get(id); ok {
		if obj, ok := value.(*txObject); ok {
			return obj
		}
	}
	return nil
}

func (e *entry) size() int {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.all.Len()
}

func (e *entry) pick() {
	e.lock.Lock()
	defer e.lock.Unlock()

	if picked, ok := e.all.Pick().Value.(*txObject); ok {
		e.all.Remove(picked.tx.ID())
	}
}

func (e *entry) delete(id thor.Bytes32) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if value, ok := e.all.Get(id); ok {
		if obj, ok := value.(*txObject); ok {
			e.all.Remove(id)
			obj.deleted = true
		}
	}
}

func (e *entry) save(obj *txObject) {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.all.Set(obj.tx.ID(), obj)
	e.dirty = true
}

func (e *entry) dumpPending(sort bool) txObjects {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.dirty {
		return nil
	}

	if sort && !e.sorted {
		Sort.Slice(e.pending, func(i, j int) bool {
			return e.pending[i].overallGP.Cmp(e.pending[j].overallGP) > 0
		})
		e.sorted = true
	}

	size := len(e.pending)
	pending := make(txObjects, size, size)

	for i, obj := range e.pending {
		pending[i] = obj
	}

	return pending
}

func (e *entry) dumpAll() txObjects {
	e.lock.Lock()
	defer e.lock.Unlock()

	all := make(txObjects, 0, e.all.Len())
	e.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			all = append(all, obj)
			return true
		}
		return false
	})

	return all
}

func (e *entry) cachePending(pending txObjects) {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.pending = pending
	e.sorted = false
	e.dirty = false
}

func (e *entry) isDirty() bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.dirty
}
